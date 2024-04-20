from __future__ import annotations

from copy import copy

from flippy.othello.board import EMPTY, PASS_MOVE, Board


class EdaxEvaluation:
    def __init__(self, depth: str, score: int, best_move: int) -> None:
        self.depth = depth
        self.score = score
        self.best_move = best_move


class EdaxEvaluations:
    def __init__(self, values: dict[Board, EdaxEvaluation]) -> None:
        self.values = values
        self._validate()

    def _validate(self) -> None:
        for board, eval in self.values.items():
            assert board.is_normalized()
            assert board.is_valid_move(eval.best_move)

    def lookup(self, board: Board) -> EdaxEvaluation:
        if board.is_game_end():
            return self._lookup_game_end(board)
        if not board.has_moves():
            return self._lookup_passed(board)

        key, rotation = board.normalized()
        value = copy(self.values[key])
        value.best_move = Board.unrotate_move(value.best_move, rotation)
        return value

    def _lookup_game_end(self, board: Board) -> EdaxEvaluation:
        empties = str(board.count(EMPTY))
        score = board.get_final_score()
        return EdaxEvaluation(empties, score, PASS_MOVE)

    def _lookup_passed(self, board: Board) -> EdaxEvaluation:
        passed = board.pass_move()
        value = copy(self.lookup(passed))

        value.best_move = PASS_MOVE
        value.score *= -1
        return value

    def update(self, other: EdaxEvaluations) -> None:
        for board, evaluation in other.values.items():
            try:
                found = self.values[board]
            except KeyError:
                add_board = True
            else:
                # TODO this make depth and confidence both fields
                add_board = int(found.depth.split("@")[0]) < int(
                    evaluation.depth.split("@")[0]
                )

            if add_board:
                self.values[board] = evaluation

    def has_all_children(self, board: Board) -> bool:
        for move in board.get_moves_as_set():
            key = board.do_normalized_move(move)
            if key not in self.values:
                return False
        return True
