from __future__ import annotations

from copy import deepcopy
from typing import Iterable

from flippy.othello.game import Game
from flippy.othello.position import PASS_MOVE, Position


class EdaxRequest:
    def __init__(
        self,
        positions: Iterable[Position],
        level: int,
        *,
        source: Position | Game | None,
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
        self.validate()

    def validate(self) -> None:
        if not (0 <= self.depth <= 60):
            raise ValueError(f"Depth must be between 0 and 60, got {self.depth}")

        if self.confidence not in [73, 87, 95, 98, 99, 100]:
            raise ValueError(
                f"Confidence must be one of [73, 87, 95, 98, 99, 100], got {self.confidence}"
            )

        if not (-64 <= self.score <= 64):
            raise ValueError(f"Score must be between -64 and 64, got {self.score}")

        if not (0 <= self.level <= 60):
            raise ValueError(f"Level must be between 0 and 60, got {self.level}")

        if self.level % 2 != 0:
            raise ValueError(f"Level must be even, got {self.level}")

        dummy = deepcopy(self.position)
        for move in self.best_moves:
            if not dummy.is_valid_move(move):
                raise ValueError(f"Invalid move {move}")

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


# TODO make this behave like a dict
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

    def update(self, other: dict[Position, EdaxEvaluation]) -> None:
        for position, evaluation in other.items():
            self.add(position, evaluation)

    def get_missing_children(self, position: Position) -> set[Position]:
        children = {
            position.do_normalized_move(move) for move in position.get_moves_as_set()
        }
        return children - self.values.keys()

    def get_missing(self, positions: set[Position]) -> set[Position]:
        missing: set[Position] = set()
        for position in positions:
            try:
                self.lookup(position)
            except KeyError:
                missing.add(position)
        return missing

    def keys(self) -> set[Position]:
        return set(self.values.keys())

    def __getitem__(self, position: Position) -> EdaxEvaluation:
        return self.lookup(position)
