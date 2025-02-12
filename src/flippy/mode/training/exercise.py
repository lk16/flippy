from __future__ import annotations

from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.position import InvalidMove


class Exercise:
    def __init__(self, color: int, moves: str) -> None:
        if color not in [BLACK, WHITE]:
            raise ValueError(f'Unknown color "{color}"')

        self.color = color
        self.moves = [Board.field_to_index(field) for field in moves.strip().split()]

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

        if self.color == BLACK and len(self.moves) % 2 == 0:
            raise ValueError(
                f"Black exercise must have an odd number of moves: {self.get_moves_seq_str()}"
            )

        if self.color == WHITE and len(self.moves) % 2 == 1:
            raise ValueError(
                f"White exercise must have an even number of moves: {self.get_moves_seq_str()}"
            )

        if self.color == BLACK:
            self.skipped_initial_moves = 0
        else:
            self.skipped_initial_moves = 1

    def get_moves_seq_str(self) -> str:
        return Board.indexes_to_fields(self.moves)
