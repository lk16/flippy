from typing import Any

from flippy.mode.training.exercise import Exercise
from flippy.mode.training.exercise_list import BLACK_TREE, WHITE_TREE, get_exercises
from flippy.othello.board import Board
from flippy.othello.position import InvalidMove


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


def check_tree_integrity(prefix: str, tree: dict[str, Any], board: Board) -> None:
    if not tree:
        return

    # Track first moves to ensure uniqueness and completeness
    valid_moves = {Board.index_to_field(i) for i in board.get_moves_as_set()}
    seen_first_moves = set()

    # Check each key-value pair
    for key, value in tree.items():
        moves = key.split()
        first_move = moves[0]

        if first_move == "transposition":
            continue

        # Check for duplicate first moves
        if first_move in seen_first_moves:
            raise AssertionError(f"Duplicate first move '{first_move}' at {prefix}")
        seen_first_moves.add(first_move)

        try:
            child = board.do_move(Board.field_to_index(moves[0]))
        except InvalidMove as e:
            raise AssertionError(
                f"Invalid first move '{moves[0]}' at {prefix}: {str(e)}"
            )

        if moves[1] == "transposition":
            continue

        try:
            grand_child = child.do_move(Board.field_to_index(moves[1]))
        except InvalidMove as e:
            raise AssertionError(
                f"Invalid second move '{moves[1]}' at {prefix}: {str(e)}"
            )

        # Recursively check subtree
        next_prefix = f"{prefix} {moves[0]} {moves[1]}"

        if isinstance(value, dict):
            check_tree_integrity(next_prefix, value, grand_child)
        elif isinstance(value, str):
            check_leaf_integrity(next_prefix, value, grand_child)
        elif isinstance(value, list):
            for value_item in value:
                check_leaf_integrity(next_prefix, value_item, grand_child)
        else:
            raise AssertionError(f"Invalid subtree at {prefix}: {type(value)}")

    # Check if all valid moves are covered
    missing_moves = valid_moves - seen_first_moves
    if missing_moves:
        raise AssertionError(
            f"Missing valid first moves at {prefix}: {sorted(missing_moves)}"
        )


def check_leaf_integrity(prefix: str, value: str, board: Board) -> None:
    if "transposition" in value:
        # TODO validate transposition
        return

    moves = value.split()

    if len(moves) == 0:
        return

    if len(moves) != 2:
        raise AssertionError(f"Invalid leaf move '{value}' at {prefix}")

    try:
        child = board.do_move(Board.field_to_index(moves[0]))
    except InvalidMove as e:
        raise AssertionError(f"Invalid first move '{moves[0]}' at {prefix}: {str(e)}")

    try:
        child.do_move(Board.field_to_index(moves[1]))
    except InvalidMove as e:
        raise AssertionError(f"Invalid second move '{moves[1]}' at {prefix}: {str(e)}")


def test_black_tree_integrity() -> None:
    assert len(BLACK_TREE) == 1, "Black tree should have exactly one root"

    key = next(iter(BLACK_TREE.keys()))

    board = Board.start().do_move(Board.field_to_index(key))
    check_tree_integrity(key, BLACK_TREE[key], board)


def test_white_tree_integrity() -> None:
    board = Board.start()
    check_tree_integrity("", WHITE_TREE, board)
