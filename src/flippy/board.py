from __future__ import annotations
from copy import deepcopy
from itertools import count
from typing import Optional


ROWS = 8
COLS = 8

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
    def __init__(self, squares: list[int], turn: int) -> None:
        self.squares = squares
        self.turn = turn

    @classmethod
    def start(cls) -> Board:
        squares = [EMPTY] * ROWS * COLS
        squares[3 * ROWS + 3] = squares[4 * ROWS + 4] = WHITE
        squares[3 * ROWS + 4] = squares[4 * ROWS + 3] = BLACK
        turn = BLACK
        return Board(squares, turn)

    def is_valid_move(self, move: int) -> bool:
        return self.do_move(move) is not None

    def do_move(self, move: int) -> Optional[Board]:
        if self.squares[move] != EMPTY:
            return None

        move_y = move // COLS
        move_x = move % COLS
        flipped: list[int] = []

        for dy, dx in DIRECTIONS:
            flipped_line: list[int] = []

            for d in count(1):
                y = move_y + dy * d
                x = move_x + dx * d

                if y not in range(ROWS) or x not in range(COLS):
                    break

                s = 8 * y + x
                square = self.squares[s]

                if square == EMPTY:
                    break

                if square == self.turn:
                    if d > 1:
                        flipped += flipped_line
                        continue
                    break

                if square == opponent(self.turn):
                    flipped_line.append(s)
                    continue

        if not flipped:
            return None

        child = Board(deepcopy(self.squares), opponent(self.turn))
        child.squares[move] = self.turn

        for f in flipped:
            child.squares[f] = self.turn

        return child

    def pass_move(self) -> Board:
        return Board(deepcopy(self.squares), opponent(self.turn))

    def has_moves(self) -> bool:
        for move in range(ROWS * COLS):
            if self.is_valid_move(move):
                return True
        return False

    def is_game_end(self) -> bool:
        return not (self.has_moves() or self.pass_move().has_moves())
