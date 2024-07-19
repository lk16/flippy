import queue
from multiprocessing import Queue
from pygame.event import Event
from typing import Any

from flippy.arguments import Arguments
from flippy.db import DB, MIN_UI_SEARCH_LEVEL
from flippy.edax.process import start_evaluation
from flippy.edax.types import EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.mode.game import GameMode
from flippy.othello.board import Board


class EvaluateMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)
        self.recv_queue: Queue[EdaxResponse] = Queue()
        self.evaluations = EdaxEvaluations()
        self.db = DB()

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
        board = self.get_board()
        if self.evaluations.has_all_children(board):
            return

        board = self.get_board()
        child_positions = {child.position for child in board.get_children()}

        evaluations = self.db.lookup_edax_positions(child_positions)
        self.evaluations.update(evaluations)

        request_positions = self.evaluations.get_missing(child_positions)

        request = EdaxRequest(request_positions, MIN_UI_SEARCH_LEVEL, source=board)
        start_evaluation(request, self.recv_queue)

    def _process_recv_messages(self) -> None:
        while True:
            try:
                message = self.recv_queue.get_nowait()
            except queue.Empty:
                break

            self._process_recv_message(message)

    def _process_recv_message(self, message: EdaxResponse) -> None:
        self.evaluations.update(message.evaluations)
        self.db.update_edax_evaluations(message.evaluations)

        positions = message.request.positions
        # TODO #33 unify modes, especially searching with PGN Mode and move it back into Window

        level = message.request.level
        source = message.request.source
        next_level = level + 2

        if (
            isinstance(source, Board)
            and source == self.get_board()
            and next_level <= 32
        ):
            next_request = EdaxRequest(positions, next_level, source=source)
            start_evaluation(next_request, self.recv_queue)

    def get_ui_details(self) -> dict[str, Any]:
        self._process_recv_messages()

        evaluations: dict[int, int] = {}

        board = self.get_board()

        for move in board.get_moves_as_set():
            child = board.do_move(move)

            try:
                evaluation = self.evaluations.lookup(child.position)
            except KeyError:
                continue

            evaluations[move] = -evaluation.score

        return {"evaluations": evaluations}
