from flippy.arguments import Arguments
from multiprocessing import Queue
import multiprocessing
import queue
from typing import Any
from flippy.edax.manager import EdaxManager
from flippy.mode.game import GameMode

from pygame.event import Event


class EvaluateMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)
        self.send_queue: "Queue[tuple[Any, ...]]" = Queue()
        self.recv_queue: "Queue[dict[int, int]]" = Queue()
        self.evaluations: dict[int, int] = {}

        edax_manager = EdaxManager(self.recv_queue, self.send_queue)
        proc = multiprocessing.Process(target=edax_manager.loop)
        proc.start()

    def on_move(self, move: int) -> None:
        super().on_move(move)
        self.on_board_change()

    def on_mouse_right_click(self, event: Event) -> None:
        super().on_mouse_right_click(event)
        self.on_board_change()

    def on_board_change(self) -> None:
        self.evaluations = {}
        self.send_queue.put_nowait(("set_board", self.get_board()))

    def get_ui_details(self) -> dict[str, Any]:
        while True:
            try:
                evaluations = self.recv_queue.get_nowait()
            except queue.Empty:
                break
            else:
                self.evaluations = evaluations

        return {"evaluations": self.evaluations}
