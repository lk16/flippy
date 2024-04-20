import multiprocessing
import queue
from multiprocessing import Queue
from pygame.event import Event
from typing import Any

from flippy.arguments import Arguments
from flippy.edax.evaluations import EdaxEvaluations
from flippy.edax.manager import EdaxManager
from flippy.mode.game import GameMode


class EvaluateMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)
        self.send_queue: Queue[tuple[Any, ...]] = Queue()
        self.recv_queue: Queue[EdaxEvaluations] = Queue()
        self.all_evaluations = EdaxEvaluations({})

        edax_manager = EdaxManager(self.recv_queue, self.send_queue)
        proc = multiprocessing.Process(target=edax_manager.loop)
        proc.start()

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

        self.send_queue.put_nowait(("set_board", self.get_board()))

    def _process_recv_messages(self) -> None:
        while True:
            try:
                evaluations = self.recv_queue.get_nowait()
            except queue.Empty:
                break
            else:
                self.all_evaluations.update(evaluations)

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
