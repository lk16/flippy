from __future__ import annotations
from copy import copy
import multiprocessing
from multiprocessing import Queue
from typing import Any, Optional, cast

from flippy.edax.process import EdaxProcess
from flippy.othello.board import EMPTY, PASS_MOVE, Board
from flippy.othello.game import Game


class EdaxEvaluation:
    def __init__(self, depth: str, score: int, best_move: int) -> None:
        self.depth = depth
        self.score = score
        self.best_move = best_move


class EdaxEvaluations:
    def __init__(self, values: dict[Board, EdaxEvaluation]) -> None:
        self.values = values
        self._validate()

    def _validate(self) -> None:
        for board, eval in self.values.items():
            assert board.is_normalized()
            assert board.is_valid_move(eval.best_move)

    def lookup(self, board: Board) -> EdaxEvaluation:
        if board.is_game_end():
            return self._lookup_game_end(board)
        if not board.has_moves():
            return self._lookup_passed(board)

        key, rotation = board.normalized()
        value = copy(self.values[key])
        value.best_move = Board.unrotate_move(value.best_move, rotation)
        return value

    def _lookup_game_end(self, board: Board) -> EdaxEvaluation:
        empties = str(board.count(EMPTY))
        score = board.get_final_score()
        return EdaxEvaluation(empties, score, PASS_MOVE)

    def _lookup_passed(self, board: Board) -> EdaxEvaluation:
        passed = board.pass_move()
        value = copy(self.lookup(passed))

        value.best_move = PASS_MOVE
        value.score *= -1
        return value

    def update(self, other: EdaxEvaluations) -> None:
        # TODO worry about losing items with lower search depth
        self.values.update(other.values)

    def has_all_children(self, board: Board) -> bool:
        for move in board.get_moves_as_set():
            key = board.do_normalized_move(move)
            if key not in self.values:
                return False
        return True


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
