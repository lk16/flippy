from __future__ import annotations

from typing import Optional

from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.position import InvalidMove, NormalizedPosition


class Exercise:
    def __init__(self, color: int, moves: str) -> None:
        if color not in [BLACK, WHITE]:
            raise ValueError(f'Unknown color "{color}"')

        self.raw = moves
        self.color = color

        self.forced_move_indices: list[int] = []
        self.moves: list[int] = []

        boards = [Board.start()]
        skipped_initial_moves: Optional[int] = None

        for offset, field in enumerate(moves.strip().split()):
            if field.startswith("&"):
                if (self.color == BLACK and offset % 2 == 1) or (
                    self.color == WHITE and offset % 2 == 0
                ):
                    raise ValueError(
                        f"Cannot skip moves until move {offset + 1}: {moves}"
                    )

                field = field[1:]
                skipped_initial_moves = offset

            if field.startswith("*"):
                if (self.color == BLACK and offset % 2 == 1) or (
                    self.color == WHITE and offset % 2 == 0
                ):
                    raise ValueError(f"Cannot force move at move {offset + 1}: {moves}")

                field = field[1:]
                self.forced_move_indices.append(offset)

            move = Board.field_to_index(field)
            self.moves.append(move)

            try:
                child = boards[-1].do_move(move)
            except InvalidMove:
                raise ValueError(
                    f'Invalid move "{Board.index_to_field(move)}" in "{moves}"'
                )

            boards.append(child)

        self.boards = boards

        if skipped_initial_moves is not None:
            self.skipped_initial_moves = skipped_initial_moves
        elif self.color == BLACK:
            self.skipped_initial_moves = 0
        else:
            self.skipped_initial_moves = 1

        if self.color == BLACK and len(self.moves) % 2 == 0:
            raise ValueError(f"Black exercise must have an odd number of moves: {self}")

        if self.color == WHITE and len(self.moves) % 2 == 1:
            raise ValueError(
                f"White exercise must have an even number of moves: {self}"
            )

    def get_normalized(self, move_index: int) -> NormalizedPosition:
        return self.boards[move_index].position.normalized()

    def get_forced_move(self, board: Board, move_index: int) -> int:
        target_position = self.get_normalized(move_index + 1)

        for move in board.get_moves_as_set():
            if board.position.do_normalized_move(move) == target_position:
                return move

        raise RuntimeError("No valid forced move found")

    def get_next_board(self, board: Board, move: int, moves_done: int) -> Board:
        child = board.do_move(move)
        normalized_grand_child = self.get_normalized(moves_done + 2)

        for grand_child in child.get_children():
            if grand_child.position.normalized() == normalized_grand_child:
                return grand_child

        raise RuntimeError("No valid child board found")

    def __str__(self) -> str:
        return Board.indexes_to_fields(self.moves)

    def __repr__(self) -> str:
        return f"Exercise(color={self.color}, moves={self.raw!r})"
