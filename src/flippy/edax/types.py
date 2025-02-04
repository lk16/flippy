from __future__ import annotations

from copy import deepcopy

from flippy.book import MIN_LEARN_LEVEL
from flippy.othello.game import Game
from flippy.othello.position import PASS_MOVE, NormalizedPosition, Position


class EdaxRequest:
    def __init__(
        self,
        positions: set[NormalizedPosition],
        level: int,
        *,
        source: Position | Game | None,
    ) -> None:
        self.positions = positions
        self.level = level
        self.source = source


class EdaxResponse:
    def __init__(self, request: EdaxRequest, evaluations: EdaxEvaluations) -> None:
        self.request = request
        self.evaluations = evaluations

        for position in self.request.positions:
            if position.is_game_end() or not position.has_moves():
                continue

            assert position in self.evaluations


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

    def is_db_savable(self) -> bool:
        return self.level >= MIN_LEARN_LEVEL and self.position.is_db_savable()


# TODO add tests for EdaxEvaluations


class EdaxEvaluations:
    """
    This class represents a set of evaluated positions and exposes an interface
    similar to a dictionary.
    """

    def __init__(self) -> None:
        self.__data: dict[NormalizedPosition, EdaxEvaluation] = {}

    # --- dict like interface ---

    def keys(self) -> set[NormalizedPosition]:
        return set(self.__data.keys())

    def values(self) -> list[EdaxEvaluation]:
        return list(self.__data.values())

    def __getitem__(self, normalized: NormalizedPosition) -> EdaxEvaluation:
        if normalized.is_game_end():
            return self.__lookup_game_end(normalized)

        if not normalized.has_moves():
            return self.__lookup_passed(normalized)

        return deepcopy(self.__data[normalized])

    def update(self, other: EdaxEvaluations) -> None:
        for position, evaluation in other.__data.items():
            self[position] = evaluation

    def __setitem__(
        self, normalized: NormalizedPosition, evaluation: EdaxEvaluation
    ) -> None:
        assert evaluation.position.normalized() == normalized

        if normalized not in self.__data:
            self.__data[normalized] = evaluation

        found = self.__data[normalized]

        if evaluation.is_better_than(found):
            self.__data[normalized] = evaluation

    def __contains__(self, normalized: NormalizedPosition) -> bool:
        return normalized in self.__data

    # --- private helpers ---

    def __lookup_game_end(self, normalized: NormalizedPosition) -> EdaxEvaluation:
        position = normalized.to_position()

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

    def __lookup_passed(self, normalized: NormalizedPosition) -> EdaxEvaluation:
        passed = normalized.pass_move()
        return deepcopy(self[passed]).pass_move()

    # --- public helpers that don't modify self ---

    def get_missing_children(self, position: Position) -> set[NormalizedPosition]:
        children = {
            position.do_normalized_move(move) for move in position.get_moves_as_set()
        }
        return children - set(self.__data.keys())

    def get_missing(
        self, positions: set[NormalizedPosition]
    ) -> set[NormalizedPosition]:
        return positions - self.__data.keys()
