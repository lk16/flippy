from __future__ import annotations

import struct
from copy import copy
from typing import Iterable

from flippy.othello.bitset import BitSet

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


class InvalidMove(Exception):
    pass


class Position:
    """
    Position stores an othello position, but does not store the color of the player to move.
    This is useful for storing and loading openings from DB.
    """

    def __init__(self, me: BitSet, opp: BitSet) -> None:
        # Bitset of discs of player to move
        self.me = me

        # Bitset of discs of opponent of player to move
        self.opp = opp

    @classmethod
    def start(cls) -> Position:
        me = BitSet(1 << 28 | 1 << 35)
        opp = BitSet(1 << 27 | 1 << 36)
        return Position(me, opp)

    @classmethod
    def empty(cls) -> Position:
        return Position(BitSet(0x0), BitSet(0x0))

    @classmethod
    def from_bytes(cls, bytes_: bytes) -> Position:
        me, opp = struct.unpack("<QQ", bytes_)
        return Position(me, opp)

    def to_bytes(self) -> bytes:
        return struct.pack("<QQ", self.me, self.opp)

    def __repr__(self) -> str:
        me = self.me.as_hex()
        opp = self.opp.as_hex()
        return f"Position({me}, {opp})"

    def is_valid_move(self, move: int) -> bool:
        moves = self.get_moves()

        if moves.is_empty():
            return move == PASS_MOVE

        return moves.is_set(move)

    def get_moves_as_set(self) -> set[int]:  # TODO move to BitSet
        moves = self.get_moves()
        return {i for i in range(64) if moves.is_set(i)}

    def get_moves(self) -> BitSet:
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

    def do_move(self, move: int) -> Position:
        if move == PASS_MOVE:
            return self.pass_move()

        if (self.me | self.opp).is_set(move):
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
                if self.opp.is_set(cur):
                    s += 1
                else:
                    if (self.me.is_set(cur)) and (s >= 2):
                        for p in range(1, s):
                            f = move + (p * (8 * dy + dx))
                            flipped |= 1 << f
                    break

        if flipped == 0:
            raise InvalidMove

        opp = self.me | flipped | (1 << move)
        me = self.opp & ~opp
        return Position(me, opp)

    def do_normalized_move(self, move: int) -> Position:
        return self.do_move(move).normalized()

    def rotated(self, rotation: int) -> Position:
        me = self.me.rotated(rotation)
        opp = self.opp.rotated(rotation)
        return Position(me, opp)

    def unrotated(self, rotation: int) -> Position:
        me = self.me.unrotated(rotation)
        opp = self.opp.unrotated(rotation)
        return Position(me, opp)

    def normalize(self) -> tuple[Position, int]:
        normalized = Position(self.me, self.opp)
        rotation = 0

        for r in range(1, 8):
            me = self.me.rotated(r)
            opp = self.opp.rotated(r)

            if (me, opp) < (normalized.me, normalized.opp):
                normalized.me = me
                normalized.opp = opp
                rotation = r

        return normalized, rotation

    def normalized(self) -> Position:
        return self.normalize()[0]

    def is_normalized(self) -> bool:
        return self.normalized() == self

    def pass_move(self) -> Position:
        return Position(copy(self.opp), copy(self.me))

    def has_moves(self) -> bool:
        return self.get_moves().has_any()

    def is_game_end(self) -> bool:
        return not (self.has_moves() or self.pass_move().has_moves())

    def get_final_score(self) -> int:
        me = self.me.count()
        opp = self.opp.count()

        if me > opp:
            return 64 - (2 * opp)
        elif opp > me:
            return -64 + (2 * me)
        else:
            return 0

    def show(self) -> None:
        moves = self.get_moves()

        print("+-a-b-c-d-e-f-g-h-+")
        for y in range(8):
            print("{} ".format(y + 1), end="")

            for x in range(8):
                if self.me.is_set_2d(x, y):
                    print("○ ", end="")
                elif self.opp.is_set_2d(x, y):
                    print("● ", end="")
                elif moves.is_set_2d(x, y):
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
            raise ValueError(f'Invalid move length "{len(field)}"')

        field = field.lower()

        if field in ["--", "ps"]:
            return PASS_MOVE

        if not ("a" <= field[0] <= "h" and "1" <= field[1] <= "8"):
            raise ValueError(f'Invalid field "{field}"')

        x = ord(field[0]) - ord("a")
        y = ord(field[1]) - ord("1")
        return y * 8 + x

    @classmethod
    def fields_to_indexes(cls, fields: list[str]) -> list[int]:
        return [cls.field_to_index(field) for field in fields]

    @classmethod
    def unrotate_move(cls, move: int, rotation: int) -> int:
        if move == PASS_MOVE:
            return move

        # Convert to BitSet.
        bit_set = BitSet(1 << move)

        # Rotate back.
        unrotated = bit_set.unrotated(rotation)

        # Take lowest set bit.
        return unrotated.lowest_bit_index()

    @classmethod
    def rotate_move(cls, move: int, rotation: int) -> int:
        if move == PASS_MOVE:
            return move

        # Convert to BitSet.
        bit_set = BitSet(1 << move)

        # Rotate.
        rotated = bit_set.rotated(rotation)

        # take lowest set bit
        return rotated.lowest_bit_index()

    def __hash__(self) -> int:
        hash_tuple = (hash(self.me), hash(self.opp))
        return hash(hash_tuple)

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Position):
            raise TypeError(f"Cannot compare Position with {type(other)}")

        return self.me == other.me and self.opp == other.opp

    def get_children(self) -> list[Position]:
        return [self.do_move(move) for move in self.get_moves_as_set()]

    def to_problem(self) -> str:
        squares = ""
        for index in range(64):
            if self.me.is_set(index):
                squares += "X"
            elif self.opp.is_set(index):
                squares += "O"
            else:
                squares += "-"

        # Turn doesn't matter for search, so we always pretend black is to move.
        turn = "X"

        return "".join(squares) + " " + turn + ";\n"

    def count_discs(self) -> int:
        return (self.me | self.opp).count()

    def count_empties(self) -> int:
        return 64 - self.count_discs()
