from __future__ import annotations

from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.position import InvalidMove


def parse_moves(moves_str: str) -> tuple[list[int], list[int]]:
    if moves_str.count("|") > 1:
        raise ValueError(f'Invalid moves "{moves_str}"')

    forced_move_idices: list[int] = []
    moves: list[int] = []

    for offset, field in enumerate(moves_str.strip().split()):
        if field.startswith("*"):
            field = field[1:]
            forced_move_idices.append(offset)

        moves.append(Board.field_to_index(field))

    return (moves, forced_move_idices)


class Exercise:
    def __init__(self, color: int, moves: str) -> None:
        if color not in [BLACK, WHITE]:
            raise ValueError(f'Unknown color "{color}"')

        self.color = color
        self.moves, self.forced_move_indices = parse_moves(moves)

        boards = [Board.start()]

        for move in self.moves:
            try:
                child = boards[-1].do_move(move)
            except InvalidMove:
                boards[-1].show()
                bad_move_str = Board.index_to_field(move)
                bad_move_seq = " ".join(
                    Board.index_to_field(move) for move in self.moves
                )
                raise ValueError(f'Invalid move "{bad_move_str}" in "{bad_move_seq}"')

            boards.append(child)

        self.boards = boards

        if self.color == BLACK:
            if len(self.moves) % 2 == 0:
                raise ValueError(
                    f"Black exercise must have an odd number of moves: {self.get_moves_seq_str()}"
                )

            for index in self.forced_move_indices:
                if index % 2 != 0:
                    raise ValueError(
                        f"Black exercise cannot force move at odd index: {self.get_moves_seq_str()}"
                    )

            self.skipped_initial_moves = 0

        else:
            if len(self.moves) % 2 == 1:
                raise ValueError(
                    f"White exercise must have an even number of moves: {self.get_moves_seq_str()}"
                )

            for index in self.forced_move_indices:
                if index % 2 != 1:
                    raise ValueError(
                        f"White exercise cannot force move at even index: {self.get_moves_seq_str()}"
                    )

            self.skipped_initial_moves = 1

    def get_moves_seq_str(self) -> str:
        return Board.indexes_to_fields(self.moves)
