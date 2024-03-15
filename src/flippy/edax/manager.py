from multiprocessing import Queue
import queue
from typing import Any, Optional

from flippy.edax.process import EdaxProcess
from flippy.othello.board import Board


class EdaxManager:
    def __init__(
        self, send_queue: "Queue[dict[int, int]]", recv_queue: "Queue[tuple[str, Any]]"
    ):
        self.send_queue = send_queue
        self.recv_queue = recv_queue
        self.edax_processes: dict[int, EdaxProcess] = {}
        self.evaluations: dict[int, int] = {}

    def loop(self) -> None:
        while True:
            for move, proc in self.edax_processes.items():
                eval = proc.get_last_eval()

                if eval is None:
                    continue

                if move not in self.evaluations or self.evaluations[move] != eval:
                    self.evaluations[move] = eval
                    self.send_queue.put(self.evaluations)

            try:
                message = self._get_last_message()
            except queue.Empty:
                continue

            if message[0] == "set_board":
                self._set_searched_board(message[1])
                self.send_queue.put(self.evaluations)
            else:
                print(f"Unhandled message kind {message[0]}")

    def _get_last_message(self) -> tuple[str, Any]:
        last_message: Optional[tuple[str, Any]] = None
        while True:
            try:
                message = self.recv_queue.get(block=False)
            except queue.Empty:
                break
            else:
                last_message = message

        if not last_message:
            raise queue.Empty
        return last_message

    def _set_searched_board(self, board: Board) -> None:
        for proc in self.edax_processes.values():
            proc.kill()

        self.evaluations = {}
        self.edax_processes = {}

        for move in board.get_moves_as_set():
            child = board.do_move(move)
            proc = EdaxProcess(child)
            proc.start()
            self.edax_processes[move] = proc
