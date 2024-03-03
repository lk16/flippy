from __future__ import annotations
from copy import deepcopy
from itertools import count
from typing import Iterable, Optional


ROWS = 8
COLS = 8

BLACK = -1
WHITE = 1
EMPTY = 0
UNKNOWN = 2  # Used in watch mode
WRONG_MOVE = 3  # Used in openings training mode

PASS_MOVE = -1

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

    @classmethod
    def empty(cls) -> Board:
        squares = [EMPTY] * ROWS * COLS
        turn = BLACK
        return Board(squares, turn)

    def is_valid_move(self, move: int) -> bool:
        return self.do_move(move) is not None

    def do_move(self, move: int) -> Optional[Board]:
        if move == PASS_MOVE:
            return self.pass_move()

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

                if square not in [WHITE, BLACK]:
                    break

                if square == self.turn:
                    if d > 1:
                        flipped += flipped_line
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

    def count(self, color: int) -> int:
        return len([s for s in self.squares if s == color])

    def show(self) -> None:
        print("+-a-b-c-d-e-f-g-h-+")
        for y in range(8):
            print("{} ".format(y + 1), end="")

            for x in range(8):
                offset = (y * 8) + x
                square = self.squares[offset]

                if square == BLACK:
                    print("○ ", end="")
                elif square == WHITE:
                    print("● ", end="")
                elif self.is_valid_move(offset):
                    print("· ", end="")
                else:
                    print("  ", end="")
            print("|")
        print("+-----------------+")

    @classmethod
    def offset_to_str(cls, offset: int) -> str:
        if offset not in range(COLS * ROWS):
            raise ValueError
        return "abcdefgh"[offset % COLS] + "12345678"[offset // COLS]

    @classmethod
    def offsets_to_str(cls, offsets: Iterable[int]) -> str:
        return " ".join(cls.offset_to_str(offset) for offset in offsets)

    @classmethod
    def str_to_offset(cls, string: str) -> int:
        if len(string) != 2:
            raise ValueError(f'Invalid move length "{string}"')

        if string == "--":
            return PASS_MOVE

        if not "a" <= string[0] <= "h" or not "1" <= string[1] <= "8":
            raise ValueError(f'Invalid move "{string}"')

        move_offset_x = ord(string[0]) - ord("a")
        move_offset_y = ord(string[1]) - ord("1")
        return move_offset_y * COLS + move_offset_x

    def __hash__(self) -> int:
        return hash((tuple(self.squares), self.turn))

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Board):
            raise TypeError(f"Cannot compare Board with {type(other)}")

        return self.squares == other.squares and self.turn == other.turn
