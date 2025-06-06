from __future__ import annotations

import random
import struct
from copy import copy, deepcopy
from typing import Iterable

from flippy.book import MAX_SAVABLE_DISCS
from flippy.othello.bitset import bits_rotate, bits_unrotate

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

    def __init__(self, me: int, opp: int) -> None:
        assert me == me & 0xFFFFFFFFFFFFFFFF
        assert opp == opp & 0xFFFFFFFFFFFFFFFF

        if me & opp:
            raise ValueError("me and opp must not overlap")

        # Bitset of discs of player to move
        self.me = me

        # Bitset of discs of opponent of player to move
        self.opp = opp

    @classmethod
    def start(cls) -> Position:
        me = 1 << 28 | 1 << 35
        opp = 1 << 27 | 1 << 36
        return Position(me, opp)

    @classmethod
    def empty(cls) -> Position:
        return Position(0x0, 0x0)

    @classmethod
    def random(cls, disc_count: int) -> Position:
        if disc_count < 4 or disc_count > 64:
            raise ValueError(
                "Cannot create random position with less than 4 or more than 64 discs"
            )

        position = cls.start()

        while position.count_discs() != disc_count:
            moves = position.get_moves_as_set()

            if not moves:
                # We ran into a dead end, start over.
                position = cls.start()
                continue

            move = random.choice(list(moves))
            position = position.do_move(move)

        return position

    def __repr__(self) -> str:
        return f"Position({hex(self.me)}, {hex(self.opp)})"

    def is_valid_move(self, move: int) -> bool:
        moves = self.get_moves()

        if move == PASS_MOVE:
            return moves == 0

        assert move in range(64)

        move_bit = 1 << move
        return move_bit & moves != 0

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

    def do_move(self, move: int) -> Position:
        if move == PASS_MOVE:
            return self.pass_move()

        assert move in range(64)

        move_bit = 1 << move

        if (self.me | self.opp) & move_bit:
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

        if flipped == 0:
            raise InvalidMove

        opp = self.me | flipped | (1 << move)
        me = self.opp & ~opp
        return Position(me, opp)

    def do_normalized_move(self, move: int) -> NormalizedPosition:
        return self.do_move(move).normalized()

    def rotated(self, rotation: int) -> Position:
        me = bits_rotate(self.me, rotation)
        opp = bits_rotate(self.opp, rotation)
        return Position(me, opp)

    def unrotated(self, rotation: int) -> Position:
        me = bits_unrotate(self.me, rotation)
        opp = bits_unrotate(self.opp, rotation)
        return Position(me, opp)

    def normalize(self) -> tuple[NormalizedPosition, int]:
        candidate = Position(self.me, self.opp)
        rotation = 0

        for r in range(1, 8):
            me = bits_rotate(self.me, r)
            opp = bits_rotate(self.opp, r)

            if (me, opp) < (candidate.me, candidate.opp):
                candidate.me = me
                candidate.opp = opp
                rotation = r

        normalized = NormalizedPosition(candidate, checked=False)
        return normalized, rotation

    def normalized(self) -> NormalizedPosition:
        return self.normalize()[0]

    def is_normalized(self) -> bool:
        return self.normalized().to_position() == self

    def pass_move(self) -> Position:
        return Position(copy(self.opp), copy(self.me))

    def has_moves(self) -> bool:
        return self.get_moves() != 0

    def is_game_end(self) -> bool:
        return not (self.has_moves() or self.pass_move().has_moves())

    def get_final_score(self) -> int:
        me_count = bin(self.me).count("1")
        opp_count = bin(self.opp).count("1")

        if me_count > opp_count:
            return 64 - (2 * opp_count)
        elif opp_count > me_count:
            return -64 + (2 * me_count)
        else:
            return 0

    def show(self) -> None:
        moves = self.get_moves()

        print("+-a-b-c-d-e-f-g-h-+")
        for y in range(8):
            print("{} ".format(y + 1), end="")

            for x in range(8):
                index = (y * 8) + x
                mask = 1 << index

                if self.me & mask:
                    print("○ ", end="")
                elif self.opp & mask:
                    print("● ", end="")
                elif moves & mask:
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

        if field in ["--", "ps", "pa"]:
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

        assert move in range(64)

        # Convert to bitset.
        bitset = 1 << move

        # Rotate back.
        rotated_back = bits_unrotate(bitset, rotation)

        # Take lowest set bit.
        return (rotated_back & -rotated_back).bit_length() - 1

    @classmethod
    def rotate_move(cls, move: int, rotation: int) -> int:
        if move == PASS_MOVE:
            return move

        assert move in range(64)

        # convert to bitset
        bitset = 1 << move

        # rotate back
        rotated_back = bits_rotate(bitset, rotation)

        # take lowest set bit
        return (rotated_back & -rotated_back).bit_length() - 1

    def __hash__(self) -> int:
        raise NotImplementedError

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Position):
            raise TypeError("Cannot compare Position with non-Position")
        return self.me == other.me and self.opp == other.opp

    def get_children(self) -> list[Position]:
        return [self.do_move(move) for move in self.get_moves_as_set()]

    def get_normalized_children(self) -> set[NormalizedPosition]:
        return {self.do_normalized_move(move) for move in self.get_moves_as_set()}

    def count_discs(self) -> int:
        return bin(self.me | self.opp).count("1")

    def count_empties(self) -> int:
        return 64 - self.count_discs()

    def is_db_savable(self) -> bool:
        return self.has_moves() and self.count_discs() <= MAX_SAVABLE_DISCS


class NormalizedPosition:
    """
    This class represents a normalized position and can be converted from and to a Position.
    It is hashable and supports the bare minimum to be usable in an EdaxRequest.

    We don't inherit from Position, to make type checking easier.
    """

    # --- Conversion from and to Position ---

    def __init__(self, position: Position, checked: bool = True) -> None:
        self.__position = deepcopy(position)

        if checked and not self.__position.is_normalized():
            raise ValueError("Position must be normalized")

    def to_position(self) -> Position:
        return deepcopy(self.__position)

    # --- Conversion to and from API ---

    @classmethod
    def from_api(cls, api_str: str) -> NormalizedPosition:
        """Create Position from a 32-character hex string (16 chars each for me and opp)."""
        if len(api_str) != 32:
            raise ValueError("API string must be exactly 32 characters")
        me = int(api_str[:16], 16)
        opp = int(api_str[16:], 16)
        return NormalizedPosition(Position(me, opp))

    def to_api(self) -> str:
        """Convert normalized position to a 32-character hex string (16 chars each for me and opp)."""
        return f"{self.__position.me:016X}{self.__position.opp:016X}"

    # --- Conversion to and from bytes ---

    @classmethod
    def from_bytes(cls, bytes_: bytes) -> NormalizedPosition:
        me, opp = struct.unpack("<QQ", bytes_)
        return NormalizedPosition(Position(me, opp))

    def to_bytes(self) -> bytes:
        return struct.pack("<QQ", self.__position.me, self.__position.opp)

    # --- Hashing and equality ---

    def as_tuple(self) -> tuple[int, int]:
        return (self.__position.me, self.__position.opp)

    def __hash__(self) -> int:
        return hash(self.as_tuple())

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, NormalizedPosition):
            raise TypeError(f"Cannot compare NormalizedPosition with {type(other)}")

        return self.as_tuple() == other.as_tuple()

    # -- Functions that don't modify self ---

    def is_game_end(self) -> bool:
        return self.__position.is_game_end()

    def has_moves(self) -> bool:
        return self.__position.has_moves()

    def pass_move(self) -> NormalizedPosition:
        return self.__position.pass_move().normalized()

    def to_problem(self) -> str:
        squares = ""
        for index in range(64):
            mask = 1 << index
            if mask & self.__position.me:
                squares += "X"
            elif mask & self.__position.opp:
                squares += "O"
            else:
                squares += "-"

        # Position does not store the turn, so we pretend to always be black.
        turn = "X"

        return "".join(squares) + " " + turn + ";\n"

    def is_db_savable(self) -> bool:
        return self.__position.is_db_savable()
