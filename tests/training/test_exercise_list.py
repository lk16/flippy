from flippy.mode.training.exercise import Exercise
from flippy.mode.training.exercise_list import get_exercises


def test_exercise_validity() -> None:
    # Validation happens inside constructors in get_exercises().
    # This should not fail.
    _ = get_exercises()


def test_exercise_order() -> None:
    def sort_key(exercise: Exercise) -> tuple[int, str]:
        return (exercise.color, exercise.raw)

    exercises = get_exercises()

    assert exercises == sorted(
        exercises, key=sort_key
    ), "Exercises should be ordered first by color, then by raw"
