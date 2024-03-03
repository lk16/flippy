from __future__ import annotations

from flippy.board import BLACK, COLS, EMPTY, ROWS, WHITE, WRONG_MOVE, Board
from flippy.mode.base import BaseMode

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass


from copy import deepcopy
import json
from pathlib import Path
import random


class Exercise:
    @classmethod
    def load_moves(cls, moves: str) -> list[int]:
        return [Board.str_to_offset(move) for move in moves.split()]

    @classmethod
    def load_color(cls, color: str) -> int:
        if color == "black":
            return BLACK
        elif color == "white":
            return WHITE
        else:
            raise ValueError(f'Unknown color "{color}"')

    @classmethod
    def load_boards(cls, moves: list[int]) -> list[Board]:
        boards = [Board.start()]

        for move in moves:
            child = boards[-1].do_move(move)

            if not child:
                boards[-1].show()
                bad_move_str = Board.offset_to_str(move)
                bad_move_seq = " ".join(Board.offset_to_str(move) for move in moves)
                raise ValueError(f'Invalid move "{bad_move_str}" in "{bad_move_seq}"')

            boards.append(child)

        return boards

    def __init__(self, item: str) -> None:
        self.raw_input = [item.strip() for item in item.split("|")]
        self.color = Exercise.load_color(self.raw_input[0])
        self.moves = Exercise.load_moves(self.raw_input[1])
        self.boards = Exercise.load_boards(self.moves)

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Exercise):
            raise TypeError
        return self.color == other.color and self.moves == other.moves


class TrainingMode(BaseMode):
    def __init__(self) -> None:
        self.file = Path(Path(__file__).parent / "../../../openings.json")
        self.remaining_exercises: list[Exercise] = []

        for item in json.loads(self.file.read_text()):
            self.remaining_exercises.append(Exercise(item))

        random.shuffle(self.remaining_exercises)

        self.exercise_offset = 0
        self.move_mistakes: set[int] = set()
        self.exercise_mistakes = False

        exercise = self.remaining_exercises[self.exercise_offset]

        if exercise.color == BLACK:
            self.moves_done = 0
        else:
            self.moves_done = 1

    def get_board(self) -> Board:
        if not self.remaining_exercises:
            return Board([EMPTY] * ROWS * COLS, BLACK)

        exercise = self.remaining_exercises[self.exercise_offset]
        board = deepcopy(exercise.boards[self.moves_done])

        for offset in self.move_mistakes:
            board.squares[offset] = WRONG_MOVE

        return board

    def on_move(self, move: int) -> None:
        if not self.remaining_exercises:
            return

        exercise = self.remaining_exercises[self.exercise_offset]
        board = self.get_board()

        if not board.is_valid_move(move):
            # Invalid move
            return

        if move != exercise.moves[self.moves_done]:
            # Incorrect move.
            self.move_mistakes.add(move)
            self.exercise_mistakes = True
            return

        self.moves_done += 2
        self.move_mistakes = set()

        if len(exercise.boards) <= self.moves_done:
            # End of exercise, find next one.

            if self.exercise_mistakes:
                # Mistakes were made. Do not remove current exercise.
                self.exercise_offset = (self.exercise_offset + 1) % len(
                    self.remaining_exercises
                )

            else:
                # No mistakes, remove current exercise.

                self.remaining_exercises.pop(self.exercise_offset)

                if len(self.remaining_exercises) <= self.exercise_offset:
                    self.exercise_offset = 0

                if not self.remaining_exercises:
                    return

            self.exercise_mistakes = False

            exercise = self.remaining_exercises[self.exercise_offset]

            if exercise.color == BLACK:
                self.moves_done = 0
            else:
                self.moves_done = 1
