from __future__ import annotations
from copy import deepcopy
from itertools import count
from typing import Optional


ROWS, COLS = 8, 8

BLACK = -1
WHITE = 1
EMPTY = 0

DIRECTIONS = [
    (-1, -1),
    (-1, 0),
    (-1, 1),
    (0, -1),
    (0, 1),
    (1, -1),
    (1, 0),
    (1, 1),
]


def opponent(color: int) -> int:
    assert color in [BLACK, WHITE]
    return -color


class Board:
    def __init__(self, squares: list[list[int]], turn: int) -> None:
        self.squares = squares
        self.turn = turn

    @classmethod
    def start(cls) -> Board:
        squares = [[EMPTY] * COLS for _ in range(ROWS)]
        squares[3][3] = squares[4][4] = WHITE
        squares[3][4] = squares[4][3] = BLACK
        turn = BLACK
        return Board(squares, turn)

    def is_valid_move(self, row: int, col: int) -> bool:
        return self.do_move(row, col) is not None

    def do_move(self, row: int, col: int) -> Optional[Board]:
        if self.squares[row][col] != EMPTY:
            return None

        flipped: list[tuple[int, int]] = []

        for dr, dc in DIRECTIONS:
            flipped_line: list[tuple[int, int]] = []

            for d in count(1):
                r = row + dr * d
                c = col + dc * d

                if r not in range(ROWS) or c not in range(COLS):
                    break

                if self.squares[r][c] == EMPTY:
                    break

                if self.squares[r][c] == self.turn:
                    if d > 1:
                        flipped += flipped_line
                        continue
                    break

                if self.squares[r][c] == opponent(self.turn):
                    flipped_line.append((r, c))
                    continue

        child = Board(deepcopy(self.squares), opponent(self.turn))
        child.squares[row][col] = self.turn

        if not flipped:
            return None

        for flip_row, flip_col in flipped:
            child.squares[flip_row][flip_col] = self.turn

        return child

    def pass_move(self) -> Board:
        return Board(deepcopy(self.squares), opponent(self.turn))

    def has_moves(self) -> bool:
        for row in range(ROWS):
            for col in range(COLS):
                if self.is_valid_move(row, col):
                    return True
        return False

    def is_game_end(self) -> bool:
        passed = Board(deepcopy(self.squares), opponent(self.turn))
        return not self.has_moves() and not passed.has_moves()
