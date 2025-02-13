from flippy.mode.training.exercise import Exercise


def test_exercise_validity() -> None:
    from flippy.mode.training.exercise_list import EXERCISES

    # The list literal will run constructors. This should not fail.
    _ = EXERCISES


def test_exercise_order() -> None:
    from flippy.mode.training.exercise_list import EXERCISES

    def sort_key(exercise: Exercise) -> tuple[int, str]:
        return (exercise.color, exercise.raw)

    assert EXERCISES == sorted(
        EXERCISES, key=sort_key
    ), "Exercises should be ordered first by color, then by raw"
