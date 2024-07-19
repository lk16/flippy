from __future__ import annotations

from typing import Iterable

from flippy.othello.bitset import BitSet
from flippy.othello.position import PASS_MOVE, Position

BLACK = -1
WHITE = 1
EMPTY = 0


def opponent(color: int) -> int:
    assert color in [BLACK, WHITE]
    return -color


class Board:
    def __init__(self, position: Position, turn: int) -> None:
        assert turn in [BLACK, WHITE]

        self.position = position
        self.turn = turn

    @classmethod
    def start(cls) -> Board:
        return Board(Position.start(), BLACK)

    @classmethod
    def empty(cls) -> Board:
        return Board(Position.empty(), BLACK)

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
            me, opp = black, white
        else:
            me, opp = white, black

        return Board(Position(BitSet(me), BitSet(opp)), turn)

    def __repr__(self) -> str:
        return f"Board({self.position}, {self.turn})"

    def is_valid_move(self, move: int) -> bool:  # pragma: nocover
        return self.position.is_valid_move(move)

    def get_square(self, move: int) -> int:
        if self.position.me.is_set(move):
            return self.turn
        if self.position.opp.is_set(move):
            return opponent(self.turn)
        return EMPTY

    def black(self) -> BitSet:
        if self.turn == BLACK:
            return self.position.me
        return self.position.opp

    def white(self) -> BitSet:
        if self.turn == WHITE:
            return self.position.me
        return self.position.opp

    def get_moves_as_set(self) -> set[int]:  # pragma: nocover
        return self.position.get_moves_as_set()

    def get_moves(self) -> BitSet:  # pragma: nocover
        return self.position.get_moves()

    def do_move(self, move: int) -> Board:  # pragma: nocover
        position = self.position.do_move(move)
        turn = opponent(self.turn)
        return Board(position, turn)

    def do_normalized_move(self, move: int) -> Board:  # pragma: nocover
        child = self.do_move(move)
        return Board(child.position.normalized(), child.turn)

    def pass_move(self) -> Board:
        return self.do_move(PASS_MOVE)

    def has_moves(self) -> bool:
        return self.position.has_moves()

    def is_game_end(self) -> bool:
        return not (self.has_moves() or self.pass_move().has_moves())

    def count(self, color: int) -> int:
        assert color in [WHITE, BLACK]

        if color == WHITE:
            return self.white().count_bits()
        else:
            return self.black().count_bits()

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
    def fields_to_indexes(cls, fields: list[str]) -> list[int]:  # pragma: nocover
        return [cls.field_to_index(field) for field in fields]

    def as_tuple(self) -> tuple[Position, int]:  # pragma: nocover
        return (self.position, self.turn)

    def __hash__(self) -> int:  # pragma: nocover
        return hash(self.as_tuple())

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Board):
            raise TypeError(f"Cannot compare Board with {type(other)}")

        return self.as_tuple() == other.as_tuple()

    def get_children(self) -> list[Board]:
        return [self.do_move(move) for move in self.get_moves_as_set()]

    def get_child_positions(self) -> list[Position]:
        return [child.position for child in self.get_children()]

    def count_discs(self) -> int:
        return self.position.count_discs()

    def count_empties(self) -> int:
        return self.position.count_empties()
