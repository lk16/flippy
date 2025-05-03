import pytest

from flippy.edax.types import EdaxEvaluation, EdaxEvaluations
from flippy.othello.position import PASS_MOVE, Position

POSITION = Position.start()
NORMALIZED = POSITION.normalized()
EVALUATION = EdaxEvaluation(
    position=Position.start(),
    level=16,
    depth=10,
    confidence=100,
    score=0,
    best_moves=[19, 18],
)


def test_keys() -> None:
    evaluations = EdaxEvaluations()
    evaluations[NORMALIZED] = EVALUATION
    assert evaluations.keys() == {NORMALIZED}


def test_values() -> None:
    evaluations = EdaxEvaluations()
    evaluations[NORMALIZED] = EVALUATION
    values = evaluations.values()
    assert len(values) == 1
    assert values[0].position == EVALUATION.position
    assert values[0].level == EVALUATION.level
    assert values[0].depth == EVALUATION.depth
    assert values[0].confidence == EVALUATION.confidence
    assert values[0].score == EVALUATION.score
    assert values[0].best_moves == EVALUATION.best_moves


def test_getitem() -> None:
    evaluations = EdaxEvaluations()
    evaluations[NORMALIZED] = EVALUATION
    result = evaluations[NORMALIZED]
    assert result.position == EVALUATION.position
    assert result.level == EVALUATION.level
    assert result.depth == EVALUATION.depth
    assert result.confidence == EVALUATION.confidence
    assert result.score == EVALUATION.score
    assert result.best_moves == EVALUATION.best_moves


def test_getitem_missing() -> None:
    evaluations = EdaxEvaluations()
    evaluations[NORMALIZED] = EVALUATION
    position = Position(0xFF, 0xFF00)
    assert position.has_moves()
    normalized = position.normalized()
    with pytest.raises(KeyError):
        _ = evaluations[normalized]


def test_lookup_game_end() -> None:
    evaluations = EdaxEvaluations()
    evaluations[NORMALIZED] = EVALUATION
    position = Position(0xFF, 0x0)
    assert position.is_game_end()
    normalized = position.normalized()
    found = evaluations[normalized]
    assert found.position == position
    assert found.depth == position.count_empties()
    assert found.score == 64
    assert found.best_moves == []
    assert found.level == found.depth + (found.depth % 2)
    assert found.confidence == 100


def test_lookup_passed() -> None:
    evaluations = EdaxEvaluations()
    evaluations[NORMALIZED] = EVALUATION
    position = Position(0xFF00, 0xFF)
    assert not position.has_moves()
    assert position.pass_move().has_moves()

    passed = position.pass_move()

    evaluations[passed.normalized()] = EdaxEvaluation(
        position=passed,
        level=16,
        depth=10,
        confidence=100,
        score=64,
        best_moves=[16],
    )

    normalized = position.normalized()
    found = evaluations[normalized]
    assert found.position == position
    assert found.depth == 10
    assert found.score == -64
    assert found.best_moves == [PASS_MOVE, 16]
    assert found.level == 16
    assert found.confidence == 100


def test_update() -> None:
    other = EdaxEvaluations()
    other[NORMALIZED] = EVALUATION

    empty_evals = EdaxEvaluations()
    empty_evals.update(other)

    assert empty_evals[NORMALIZED].position == EVALUATION.position
    assert empty_evals[NORMALIZED].level == EVALUATION.level


def test_setitem_better_evaluation() -> None:
    evals = EdaxEvaluations()

    worse_eval = EdaxEvaluation(
        position=NORMALIZED.to_position(),
        level=16,
        depth=10,
        confidence=95,
        score=0,
        best_moves=[19],
    )

    better_eval = EdaxEvaluation(
        position=NORMALIZED.to_position(),
        level=16,
        depth=12,  # Higher depth
        confidence=100,  # Higher confidence
        score=0,
        best_moves=[19],
    )

    evals[NORMALIZED] = worse_eval
    evals[NORMALIZED] = better_eval

    result = evals[NORMALIZED]
    assert result.depth == better_eval.depth
    assert result.confidence == better_eval.confidence


def test_setitem_validation() -> None:
    evals = EdaxEvaluations()
    wrong_position = Position.empty()

    evaluation = EdaxEvaluation(
        position=wrong_position,
        level=16,
        depth=10,
        confidence=100,
        score=0,
        best_moves=[],
    )

    with pytest.raises(
        ValueError, match="Evaluation position .* does not match normalized .*"
    ):
        evals[NORMALIZED] = evaluation


def test_contains() -> None:
    evaluations = EdaxEvaluations()
    evaluations[NORMALIZED] = EVALUATION
    assert NORMALIZED in evaluations
    assert Position.empty().normalized() not in evaluations
