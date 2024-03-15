from __future__ import annotations
from typing import Optional
from flippy.othello.board import BLACK, WHITE, Board, InvalidMove


class Exercise:
    @classmethod
    def load_moves(cls, fields: str) -> tuple[list[int], bool]:
        has_skipped_children = False
        if fields.endswith("..."):
            fields = fields[:-3].strip()
            has_skipped_children = True

        moves = [Board.field_to_index(field) for field in fields.split()]

        return moves, has_skipped_children

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
            try:
                child = boards[-1].do_move(move)
            except InvalidMove:
                boards[-1].show()
                bad_move_str = Board.index_to_field(move)
                bad_move_seq = " ".join(Board.index_to_field(move) for move in moves)
                raise ValueError(f'Invalid move "{bad_move_str}" in "{bad_move_seq}"')

            boards.append(child)

        return boards

    @classmethod
    def load_eval(cls, raw_eval: str) -> Optional[int]:
        if raw_eval == "?":
            return None
        eval = int(raw_eval)
        assert eval % 2 == 0
        assert -64 <= eval <= 64
        return eval

    def __init__(self, raw: dict[str, str]) -> None:
        self.raw = raw
        self.color = Exercise.load_color(self.raw["color"])
        self.moves, self.has_skipped_children = Exercise.load_moves(self.raw["moves"])
        self.boards = Exercise.load_boards(self.moves)
        self.eval = Exercise.load_eval(self.raw["eval"])

        if self.color == BLACK:
            self.skipped_initial_moves = 0
        else:
            self.skipped_initial_moves = 1

        assert self.raw["interest"] in ["vhigh", "high", "mid", "low", "vlow", "tp"]

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Exercise):
            raise TypeError
        return self.color == other.color and self.moves == other.moves

    def get_moves_seq_str(self) -> str:
        return Board.indexes_to_fields(self.moves)
