import pytest

from flippy.mode.training.exercise import Exercise
from flippy.othello.board import BLACK, WHITE

VALID_BLACK_EXERCISES: list[tuple[str, list[int], list[int]]] = [
    # (moves, expected_moves_indices, expected_forced_moves)
    ("e6", [44], []),
    ("*e6 d6 c5", [44, 43, 34], [0]),
    ("&e6 d6 c5", [44, 43, 34], []),
    ("&*e6 d6 c5", [44, 43, 34], [0]),
]

VALID_WHITE_EXERCISES: list[tuple[str, list[int], list[int]]] = [
    # (moves, expected_moves_indices, expected_forced_moves)
    ("e6 d6", [44, 43], []),
    ("e6 *d6 c5 f4", [44, 43, 34, 29], [1]),
    ("e6 &d6 c5 f4", [44, 43, 34, 29], []),
    ("e6 &*d6 c5 f4", [44, 43, 34, 29], [1]),
]

INVALID_EXERCISES: list[tuple[int, str, str]] = [
    # (color, moves, expected_error_message)
    (BLACK, "d3 e3", "Black exercise must have an odd number of moves"),
    (WHITE, "d3", "White exercise must have an even number of moves"),
    (WHITE, "*d3", "Cannot force move at move 1"),
    (BLACK, "d3 *e3", "Cannot force move at move 2"),
    (WHITE, "&e3", "Cannot skip moves until move 1"),
    (BLACK, "d3 &e3", "Cannot skip moves until move 2"),
    (BLACK, "xx", 'Invalid field "xx"'),
    (3, "d3", 'Unknown color "3"'),
]


@pytest.mark.parametrize(
    "moves, expected_moves, expected_forced", VALID_BLACK_EXERCISES
)
def test_valid_black_exercises(
    moves: str, expected_moves: list[int], expected_forced: list[int]
) -> None:
    exercise = Exercise(BLACK, moves)
    assert exercise.moves == expected_moves
    assert exercise.forced_move_indices == expected_forced
    assert str(exercise) == moves.replace("*", "").replace("&", "")


@pytest.mark.parametrize(
    "moves, expected_moves, expected_forced", VALID_WHITE_EXERCISES
)
def test_valid_white_exercises(
    moves: str, expected_moves: list[int], expected_forced: list[int]
) -> None:
    exercise = Exercise(WHITE, moves)
    assert exercise.moves == expected_moves
    assert exercise.forced_move_indices == expected_forced
    assert str(exercise) == moves.replace("*", "").replace("&", "")


@pytest.mark.parametrize("color, moves, error_message", INVALID_EXERCISES)
def test_invalid_exercises(color: int, moves: str, error_message: str) -> None:
    with pytest.raises(ValueError, match=error_message):
        Exercise(color, moves)


def test_skipped_initial_moves() -> None:
    # Test default skipped moves
    black_exercise = Exercise(BLACK, "e6 d6 c5")
    assert black_exercise.skipped_initial_moves == 0

    white_exercise = Exercise(WHITE, "e6 d6 c5 f4")
    assert white_exercise.skipped_initial_moves == 1

    # Test explicit skipped moves
    black_exercise_skip = Exercise(BLACK, "e6 d6 &c5")
    assert black_exercise_skip.skipped_initial_moves == 2

    white_exercise_skip = Exercise(WHITE, "e6 d6 c5 &f4")
    assert white_exercise_skip.skipped_initial_moves == 3
