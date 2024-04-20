from __future__ import annotations

import multiprocessing
from multiprocessing import Queue
from typing import Any, Optional, cast

from flippy.edax.evaluations import EdaxEvaluations
from flippy.edax.process import EdaxProcess
from flippy.othello.board import Board
from flippy.othello.game import Game


class EdaxManager:
    def __init__(
        self,
        send_queue: Queue[EdaxEvaluations],
        recv_queue: Queue[tuple[Any, ...]],
    ):
        self.send_queue = send_queue
        self.recv_queue = recv_queue
        self.loop_running = False
        self.searching: Optional[Board] = None

    def loop(self) -> None:
        self.loop_running = True

        handlers = {
            "set_board": self._handle_set_board,
            "set_game": self._handle_set_game,
            "evaluations": self._handle_evaluations,
        }

        while True:
            message = self.recv_queue.get()
            message_type: str = message[0]

            try:
                handler = handlers[message_type]
            except KeyError:
                print(f"Unhandled message kind {message_type}")
            else:
                handler(message)

    def _handle_set_board(self, message: tuple[Any, ...]) -> None:
        _, board = cast(tuple[str, Board], message)

        self.searching = board
        children = board.get_children()

        proc = EdaxProcess(children, 4, self.recv_queue, self.searching)
        multiprocessing.Process(target=proc.search).start()

    def _handle_set_game(self, message: tuple[Any, ...]) -> None:
        _, game = cast(tuple[str, Game], message)

        self.searching = None
        children = game.get_all_children()

        proc = EdaxProcess(children, 4, self.recv_queue, self.searching)
        multiprocessing.Process(target=proc.search).start()

    def _handle_evaluations(self, message: tuple[Any, ...]) -> None:
        _, parent, level, evaluations = cast(
            tuple[str, Optional[Board], int, EdaxEvaluations], message
        )

        self.send_queue.put_nowait(evaluations)

        next_level = level + 2

        if parent is not None and parent == self.searching and level <= 24:
            children = self.searching.get_children()
            proc = EdaxProcess(children, next_level, self.recv_queue, self.searching)
            multiprocessing.Process(target=proc.search).start()
