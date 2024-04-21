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
    def __init__(
        self,
        board: Board,
        depth: int,
        confidence: int,
        score: int,
        best_moves: list[int],
    ) -> None:
        self.depth = depth
        self.confidence = confidence
        self.score = score
        self.best_moves = best_moves
        self._validate(board)

    def _validate(self, board: Board) -> None:
        assert 0 <= self.depth <= 60
        assert self.confidence in [73, 87, 95, 98, 99, 100]
        assert -64 <= self.score <= 64

        for move in self.best_moves:
            assert board.is_valid_move(move)
            board = board.do_move(move)

    def is_better_than(self, other: EdaxEvaluation) -> bool:
        return (self.depth, self.confidence) > (other.depth, other.confidence)


class EdaxEvaluations:
    def __init__(self) -> None:
        self.__values: dict[Board, EdaxEvaluation] = {}

    def lookup(self, board: Board) -> EdaxEvaluation:
        if board.is_game_end():
            return self._lookup_game_end(board)
        if not board.has_moves():
            return self._lookup_passed(board)

        key, rotation = board.normalized()
        value = copy(self.__values[key])
        value.best_moves = [
            Board.unrotate_move(move, rotation) for move in value.best_moves
        ]
        return value

    def _lookup_game_end(self, board: Board) -> EdaxEvaluation:
        empties = board.count(EMPTY)
        score = board.get_final_score()
        return EdaxEvaluation(board, empties, 100, score, [])

    def _lookup_passed(self, board: Board) -> EdaxEvaluation:
        passed = board.pass_move()
        value = copy(self.lookup(passed))

        value.best_moves = [PASS_MOVE]
        value.score *= -1
        return value

    def add(self, board: Board, evaluation: EdaxEvaluation) -> None:
        assert board.is_normalized()

        if board not in self.__values:
            self.__values[board] = evaluation

        found = self.__values[board]

        if evaluation.is_better_than(found):
            self.__values[board] = evaluation

    def update(self, other: EdaxEvaluations) -> None:
        for board, evaluation in other.__values.items():
            self.add(board, evaluation)

    def has_all_children(self, board: Board) -> bool:
        children = {board.do_normalized_move(move) for move in board.get_moves_as_set()}
        return all(child in self.__values for child in children)
