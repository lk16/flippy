import queue
from multiprocessing import Queue
from pygame.event import Event
from typing import Any

from flippy.arguments import Arguments
from flippy.book import MAX_UI_SEARCH_LEVEL, MIN_UI_SEARCH_LEVEL
from flippy.book.api_client import APIClient
from flippy.edax.process import start_evaluation
from flippy.edax.types import EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.mode.game import GameMode
from flippy.othello.position import Position


class EvaluateMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)
        self.recv_queue: Queue[EdaxResponse] = Queue()
        self.evaluations = EdaxEvaluations()
        self.api_client = APIClient()

        self.on_board_change()

    def on_move(self, move: int) -> None:
        board = self.get_board()
        super().on_move(move)
        if self.get_board() != board:
            self.on_board_change()

    def on_mouse_right_click(self, event: Event) -> None:
        board = self.get_board()
        super().on_mouse_right_click(event)
        if self.get_board() != board:
            self.on_board_change()

    def on_board_change(self) -> None:
        position = self.get_board().position

        # Get positions that are missing and not currently being evaluated
        missing_positions = self.evaluations.get_missing_children(position)

        if missing_positions:
            found_evaluations = self.api_client.lookup_positions(missing_positions)
            self.evaluations.update(found_evaluations)

        self._search_missing_positions(position, MIN_UI_SEARCH_LEVEL)

    def _process_recv_messages(self) -> None:
        while True:
            try:
                message = self.recv_queue.get_nowait()
            except queue.Empty:
                break

            self._process_recv_message(message)

    def _process_recv_message(self, message: EdaxResponse) -> None:
        print(f"Received evaluations for level {message.request.level}")
        self.evaluations.update(message.evaluations)

        # Submit evaluations to server API
        savable_evaluations = [
            eval for eval in message.evaluations.values() if eval.is_db_savable()
        ]

        self.api_client.save_learned_evaluations(savable_evaluations)

        source = message.request.source

        assert isinstance(source, Position)
        if source != self.get_board().position:
            # Board changed, don't evaluate further
            return

        next_level = message.request.level + 2
        self._search_missing_positions(source, next_level)

    def _search_missing_positions(self, source: Position, level: int) -> None:
        if level > MAX_UI_SEARCH_LEVEL:
            return

        learn_positions = set()
        for position in source.get_normalized_children():
            if position not in self.evaluations:
                learn_positions.add(position)
                continue

            evaluation = self.evaluations[position]

            if evaluation.level < level:
                learn_positions.add(position)

        if not learn_positions:
            self._search_missing_positions(source, level + 2)
            return

        request = EdaxRequest(learn_positions, level, source=source)
        start_evaluation(request, self.recv_queue)

    def get_ui_details(self) -> dict[str, Any]:
        self._process_recv_messages()

        evaluations: dict[int, dict[str, int]] = {}

        board = self.get_board()

        for move in board.get_moves_as_set():
            child = board.do_move(move)

            try:
                evaluation = self.evaluations[child.position.normalized()]
            except KeyError:
                continue

            evaluations[move] = {
                "score": -evaluation.score,
                "level": evaluation.level,
            }

        return {"evaluations": evaluations}
