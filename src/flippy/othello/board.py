from __future__ import annotations
from copy import copy
from typing import Iterable

from flippy.othello.bitset import bits_rotate


BLACK = -1
WHITE = 1
EMPTY = 0

PASS_MOVE = -1
SKIPPED_CHILDREN = -2  # used in training mode

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


class InvalidMove(Exception):
    pass


class Board:
    def __init__(self, me: int, opp: int, turn: int) -> None:
        assert me == me & 0xFFFFFFFFFFFFFFFF
        assert opp == opp & 0xFFFFFFFFFFFFFFFF
        assert turn in [BLACK, WHITE]

        self.me = me
        self.opp = opp
        self.turn = turn

    @classmethod
    def start(cls) -> Board:
        me = 1 << 28 | 1 << 35
        opp = 1 << 27 | 1 << 36
        return Board(me, opp, BLACK)

    @classmethod
    def empty(cls) -> Board:
        return Board(0x0, 0x0, BLACK)

    @classmethod
    def from_squares(cls, squares: list[int], turn: int) -> Board:
        assert len(squares) == 64
        assert turn in [BLACK, WHITE]

        white = 0
        black = 0
        for index, square in enumerate(squares):
            mask = 1 << index
            if square == WHITE:
                white |= mask
            elif square == BLACK:
                black |= mask

        if turn == BLACK:
            return Board(black, white, BLACK)
        return Board(white, black, WHITE)

    def __repr__(self) -> str:
        return f"Board({hex(self.me)}, {hex(self.opp)}, {self.turn})"

    def is_valid_move(self, move: int) -> bool:
        return move in self.get_moves_as_set()

    def get_square(self, move: int) -> int:
        assert move in range(64)
        mask = 1 << move
        if self.me & mask:
            return self.turn
        if self.opp & mask:
            return opponent(self.turn)
        return EMPTY

    def black(self) -> int:
        if self.turn == BLACK:
            return self.me
        return self.opp

    def white(self) -> int:
        if self.turn == WHITE:
            return self.me
        return self.opp

    def get_moves_as_set(self) -> set[int]:
        moves = self.get_moves()
        return {i for i in range(64) if (1 << i) & moves}

    def get_moves(self) -> int:
        mask = self.opp & 0x7E7E7E7E7E7E7E7E

        flipL = mask & (self.me << 1)
        flipL |= mask & (flipL << 1)
        maskL = mask & (mask << 1)
        flipL |= maskL & (flipL << (2 * 1))
        flipL |= maskL & (flipL << (2 * 1))
        flipR = mask & (self.me >> 1)
        flipR |= mask & (flipR >> 1)
        maskR = mask & (mask >> 1)
        flipR |= maskR & (flipR >> (2 * 1))
        flipR |= maskR & (flipR >> (2 * 1))
        movesSet = (flipL << 1) | (flipR >> 1)

        flipL = mask & (self.me << 7)
        flipL |= mask & (flipL << 7)
        maskL = mask & (mask << 7)
        flipL |= maskL & (flipL << (2 * 7))
        flipL |= maskL & (flipL << (2 * 7))
        flipR = mask & (self.me >> 7)
        flipR |= mask & (flipR >> 7)
        maskR = mask & (mask >> 7)
        flipR |= maskR & (flipR >> (2 * 7))
        flipR |= maskR & (flipR >> (2 * 7))
        movesSet |= (flipL << 7) | (flipR >> 7)

        flipL = mask & (self.me << 9)
        flipL |= mask & (flipL << 9)
        maskL = mask & (mask << 9)
        flipL |= maskL & (flipL << (2 * 9))
        flipL |= maskL & (flipL << (2 * 9))
        flipR = mask & (self.me >> 9)
        flipR |= mask & (flipR >> 9)
        maskR = mask & (mask >> 9)
        flipR |= maskR & (flipR >> (2 * 9))
        flipR |= maskR & (flipR >> (2 * 9))
        movesSet |= (flipL << 9) | (flipR >> 9)

        flipL = self.opp & (self.me << 8)
        flipL |= self.opp & (flipL << 8)
        maskL = self.opp & (self.opp << 8)
        flipL |= maskL & (flipL << (2 * 8))
        flipL |= maskL & (flipL << (2 * 8))
        flipR = self.opp & (self.me >> 8)
        flipR |= self.opp & (flipR >> 8)
        maskR = self.opp & (self.opp >> 8)
        flipR |= maskR & (flipR >> (2 * 8))
        flipR |= maskR & (flipR >> (2 * 8))
        movesSet |= (flipL << 8) | (flipR >> 8)

        return movesSet & ~(self.me | self.opp) & 0xFFFFFFFFFFFFFFFF

    def do_move(self, move: int) -> Board:
        if move == PASS_MOVE:
            return self.pass_move()

        if self.get_square(move) != EMPTY:
            raise InvalidMove

        moves = self.get_moves()
        if moves & (1 << move) == 0:
            raise InvalidMove

        flipped = 0
        for dx, dy in DIRECTIONS:
            s = 1
            while True:
                curx = int(move % 8) + (dx * s)
                cury = int(move / 8) + (dy * s)
                if curx < 0 or curx >= 8 or cury < 0 or cury >= 8:
                    break

                cur = 8 * cury + curx
                if self.opp & (1 << cur):
                    s += 1
                else:
                    if (self.me & (1 << cur)) and (s >= 2):
                        for p in range(1, s):
                            f = move + (p * (8 * dy + dx))
                            flipped |= 1 << f
                    break

        opp = self.me | flipped | (1 << move)
        me = self.opp & ~opp
        turn = opponent(self.turn)
        return Board(me, opp, turn)

    def do_normalized_move(self, move: int) -> Board:
        return self.do_move(move).normalized()[0]

    def rotated(self, rotation: int) -> Board:
        me = bits_rotate(self.me, rotation)
        opp = bits_rotate(self.opp, rotation)
        return Board(me, opp, self.turn)

    def normalized(self) -> tuple[Board, int]:
        normalized = Board(self.me, self.opp, self.turn)
        rotation = 0

        for r in range(1, 8):
            me = bits_rotate(self.me, r)
            opp = bits_rotate(self.opp, r)

            if (me, opp) < (normalized.me, normalized.opp):
                normalized.me = me
                normalized.opp = opp
                rotation = r

        return normalized, rotation

    def pass_move(self) -> Board:
        return Board(copy(self.opp), copy(self.me), opponent(self.turn))

    def has_moves(self) -> bool:
        for move in range(64):
            if self.is_valid_move(move):
                return True
        return False

    def is_game_end(self) -> bool:
        return not (self.has_moves() or self.pass_move().has_moves())

    def count(self, color: int) -> int:
        assert color in [WHITE, BLACK]

        if color == WHITE:
            return bin(self.white()).count("1")
        return bin(self.black()).count("1")

    def show(self) -> None:
        print("+-a-b-c-d-e-f-g-h-+")
        for y in range(8):
            print("{} ".format(y + 1), end="")

            for x in range(8):
                index = (y * 8) + x
                square = self.get_square(index)

                if square == BLACK:
                    print("○ ", end="")
                elif square == WHITE:
                    print("● ", end="")
                elif self.is_valid_move(index):
                    print("· ", end="")
                else:
                    print("  ", end="")
            print("|")
        print("+-----------------+")

    @classmethod
    def index_to_field(cls, index: int) -> str:
        if index not in range(64):
            raise ValueError
        return "abcdefgh"[index % 8] + "12345678"[index // 8]

    @classmethod
    def indexes_to_fields(cls, indexes: Iterable[int]) -> str:
        return " ".join(cls.index_to_field(index) for index in indexes)

    @classmethod
    def field_to_index(cls, field: str) -> int:
        if len(field) != 2:
            raise ValueError(f'Invalid move length "{field}"')

        if field == "--":
            return PASS_MOVE

        field = field.lower()

        if not ("a" <= field[0] <= "h" and "1" <= field[1] <= "8"):
            raise ValueError(f'Invalid field "{field}"')

        x = ord(field[0]) - ord("a")
        y = ord(field[1]) - ord("1")
        return y * 8 + x

    def as_tuple(self) -> tuple[int, int, int]:
        return (self.me, self.opp, self.turn)

    def __hash__(self) -> int:
        return hash(self.as_tuple())

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Board):
            raise TypeError(f"Cannot compare Board with {type(other)}")

        return self.as_tuple() == other.as_tuple()

    def to_fen(self) -> str:
        empties_mask = ~(self.white() | self.black())
        rows = []

        for y in range(8):
            row = ""
            empties_count = 0
            for x in range(8):
                index = y * 8 + x
                mask = 1 << index

                if empties_mask & mask:
                    empties_count += 1
                    continue

                if empties_count != 0:
                    row += f"{empties_count}"
                empties_count = 0

                if self.white() & mask:
                    row += "P"
                else:
                    row += "p"

            if empties_count != 0:
                row += f"{empties_count}"

            rows.append(row)

        if self.turn == WHITE:
            turn = "w"
        else:
            turn = "b"

        # For some reason FEN rows are listed in bottom-to-top order.
        return "/".join(reversed(rows)) + f" {turn}"

    def to_problem(self) -> str:
        square_values = {
            EMPTY: "-",
            BLACK: "X",
            WHITE: "O",
        }

        squares = [square_values[self.get_square(index)] for index in range(64)]
        turn = square_values[self.turn]

        return "".join(squares) + " " + turn + ";\n"
