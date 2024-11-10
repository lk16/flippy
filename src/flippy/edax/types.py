from __future__ import annotations

from copy import deepcopy
from typing import Iterable

from flippy.othello.board import Board
from flippy.othello.game import Game
from flippy.othello.position import PASS_MOVE, Position


class EdaxRequest:
    def __init__(
        self,
        positions: Iterable[Position],
        level: int,
        *,
        source: Board | Game | None,
    ) -> None:
        self.positions = set(positions)
        self.level = level
        self.source = source


class EdaxResponse:
    def __init__(self, request: EdaxRequest, evaluations: "EdaxEvaluations") -> None:
        self.request = request
        self.evaluations = evaluations

        for position in self.request.positions:
            if position.is_game_end() or not position.has_moves():
                continue

            assert position.normalized() in evaluations.values


class EdaxEvaluation:
    def __init__(
        self,
        *,
        position: Position,
        level: int,
        depth: int,
        confidence: int,
        score: int,
        best_moves: list[int],
    ) -> None:
        self.position = position
        self.level = level
        self.depth = depth
        self.confidence = confidence
        self.score = score
        self.best_moves = best_moves
        self._validate()

    def _validate(self) -> None:
        assert 0 <= self.depth <= 60
        assert self.confidence in [73, 87, 95, 98, 99, 100]
        assert -64 <= self.score <= 64
        assert 0 <= self.level <= 60
        # TODO assert self.level % 2 == 0

        dummy = deepcopy(self.position)
        for move in self.best_moves:
            assert dummy.is_valid_move(move)
            dummy = dummy.do_move(move)

    def is_better_than(self, other: EdaxEvaluation) -> bool:
        return (self.depth, self.confidence) > (other.depth, other.confidence)

    def unrotate(self, rotation: int) -> EdaxEvaluation:
        best_moves = [
            Position.unrotate_move(move, rotation) for move in self.best_moves
        ]

        position = self.position.unrotated(rotation)

        return EdaxEvaluation(
            position=position,
            level=self.level,
            depth=self.depth,
            confidence=self.confidence,
            score=self.score,
            best_moves=best_moves,
        )

    def pass_move(self) -> EdaxEvaluation:
        return EdaxEvaluation(
            position=self.position.pass_move(),
            level=self.level,
            depth=self.depth,
            confidence=self.confidence,
            score=-self.score,
            best_moves=[PASS_MOVE] + self.best_moves,
        )


class EdaxEvaluations:
    def __init__(self) -> None:
        self.values: dict[Position, EdaxEvaluation] = {}

    def lookup(self, position: Position) -> EdaxEvaluation:
        if position.is_game_end():
            return self._lookup_game_end(position)

        if not position.has_moves():
            return self._lookup_passed(position)

        key, rotation = position.normalize()
        return deepcopy(self.values[key]).unrotate(rotation)

    def _lookup_game_end(self, position: Position) -> EdaxEvaluation:
        empties = position.count_empties()
        score = position.get_final_score()
        return EdaxEvaluation(
            position=position,
            depth=empties,
            level=empties + (empties % 2),
            confidence=100,
            score=score,
            best_moves=[],
        )

    def _lookup_passed(self, position: Position) -> EdaxEvaluation:
        passed = position.pass_move()
        return deepcopy(self.lookup(passed)).pass_move()

    def add(self, position: Position, evaluation: EdaxEvaluation) -> None:
        assert position.is_normalized()
        assert evaluation.position == position

        if position not in self.values:
            self.values[position] = evaluation

        found = self.values[position]

        if evaluation.is_better_than(found):
            self.values[position] = evaluation

    def update(self, other: EdaxEvaluations) -> None:
        for board, evaluation in other.values.items():
            self.add(board, evaluation)

    def has_all_children(self, board: Board) -> bool:
        children = {board.do_normalized_move(move) for move in board.get_moves_as_set()}
        return all(child in self.values for child in children)

    def get_missing(self, positions: set[Position]) -> set[Position]:
        missing: set[Position] = set()
        for position in positions:
            try:
                self.lookup(position)
            except KeyError:
                missing.add(position)
        return missing
