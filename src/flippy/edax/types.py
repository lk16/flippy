from __future__ import annotations

from copy import copy

from flippy.othello.board import EMPTY, PASS_MOVE, Board
from flippy.othello.game import Game


class EdaxRequest:
    def __init__(self, task: Board | Game, level: int) -> None:
        self.task = task
        self.level = level


class EdaxResponse:
    def __init__(self, request: EdaxRequest, evaluations: EdaxEvaluations) -> None:
        self.request = request
        self.evaluations = evaluations


class EdaxEvaluation:
    def __init__(self, depth: int, confidence: int, score: int, best_move: int) -> None:
        self.depth = depth
        self.confidence = confidence
        self.score = score
        self.best_move = best_move

    def is_better_than(self, other: EdaxEvaluation) -> bool:
        return (self.depth, self.confidence) > (other.depth, other.confidence)


class EdaxEvaluations:
    def __init__(self, values: dict[Board, EdaxEvaluation]) -> None:
        self.values = values
        self._validate()

    def _validate(self) -> None:
        for board, eval in self.values.items():
            assert board.is_normalized()

            if eval.best_move == PASS_MOVE:  # TODO remove
                return

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
        empties = board.count(EMPTY)
        score = board.get_final_score()
        return EdaxEvaluation(empties, 100, score, PASS_MOVE)

    def _lookup_passed(self, board: Board) -> EdaxEvaluation:
        passed = board.pass_move()
        value = copy(self.lookup(passed))

        value.best_move = PASS_MOVE
        value.score *= -1
        return value

    def update(self, other: EdaxEvaluations) -> None:
        for board, evaluation in other.values.items():
            if board not in self.values:
                self.values[board] = evaluation
                continue

            found = self.values[board]

            if evaluation.is_better_than(found):
                self.values[board] = evaluation

    def has_all_children(self, board: Board) -> bool:
        for move in board.get_moves_as_set():
            key = board.do_normalized_move(move)
            if key not in self.values:
                return False
        return True
