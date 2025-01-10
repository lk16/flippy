from __future__ import annotations

from typing import Any

from flippy.arguments import Arguments
from flippy.db import DB, PositionNotFound
from flippy.mode.base import BaseMode
from flippy.othello.board import Board


class NoExercisesLeft(Exception):
    pass


class ChallengeMode(BaseMode):
    def __init__(self, args: Arguments) -> None:
        _ = args

        self.excercises: list[Board] = [
            Board.start().do_move(44).do_move(43),  # parallel opening
            Board.start().do_move(44).do_move(45),  # diagonal opening
            Board.start().do_move(44).do_move(29),  # perpendicular opening
        ]

        self.move_mistakes: set[int] = set()
        self.db = DB()

    def get_board(self) -> Board:
        try:
            return self.excercises[0]
        except IndexError:
            return Board.empty()

    def get_ui_details(self) -> dict[str, Any]:
        return {"move_mistakes": self.move_mistakes}

    def is_ok_board(self, board: Board) -> bool:
        if len(board.get_children()) < 2:
            return False

        for child in board.get_children():
            try:
                self.db.lookup_edax_position(child.position)
            except PositionNotFound:
                return False

        return True

    def on_move(self, move: int) -> None:
        try:
            board = self.get_board()
        except IndexError:
            return

        if not board.is_valid_move(move):
            # Invalid move
            return

        best_move = -1
        best_score = -1000

        for m in board.get_moves_as_set():
            child = board.do_move(m)
            try:
                evaluation = self.db.lookup_edax_position(child.position)
            except PositionNotFound:
                continue

            score = -evaluation.score
            if score > best_score:
                best_score = score
                best_move = m

        if move != best_move:
            # Incorrect move.
            self.move_mistakes.add(move)
            return

        if not self.move_mistakes:
            self.excercises.extend(board.get_children())

        self.move_mistakes = set()

        # Pop board we just solved.
        self.excercises.pop(0)

        # Pop boring boards and boards that are not in the opening book, so we don't pollute the db.
        while self.excercises and not self.is_ok_board(self.excercises[0]):
            self.excercises.pop(0)
