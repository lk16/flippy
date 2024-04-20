import queue
from multiprocessing import Queue
from pygame.event import Event
from typing import Any

from flippy.arguments import Arguments
from flippy.edax.process import start_evaluation
from flippy.edax.types import EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.mode.game import GameMode
from flippy.othello.game import Game


class EvaluateMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)
        self.recv_queue: Queue[EdaxResponse] = Queue()
        self.all_evaluations = EdaxEvaluations({})
        request = EdaxRequest(self.get_board(), 2)
        start_evaluation(request, self.recv_queue)

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
        if self.all_evaluations.has_all_children(board):
            return

        request = EdaxRequest(self.get_board(), 2)
        start_evaluation(request, self.recv_queue)

    def _process_recv_messages(self) -> None:
        while True:
            try:
                message = self.recv_queue.get_nowait()
            except queue.Empty:
                break

            self._process_recv_message(message)

    def _process_recv_message(self, message: EdaxResponse) -> None:
        self.all_evaluations.update(message.evaluations)

        task = message.request.task
        level = message.request.level

        if isinstance(task, Game):
            return

        next_level = level + 2

        if self.get_board() == task and next_level <= 24:
            next_request = EdaxRequest(task, next_level)
            start_evaluation(next_request, self.recv_queue)

    def get_ui_details(self) -> dict[str, Any]:
        self._process_recv_messages()

        evaluations: dict[int, int] = {}

        board = self.get_board()

        for move in board.get_moves_as_set():
            child = board.do_move(move)

            try:
                evaluation = self.all_evaluations.lookup(child)
            except KeyError:
                continue

            evaluations[move] = -evaluation.score

        return {"evaluations": evaluations}
