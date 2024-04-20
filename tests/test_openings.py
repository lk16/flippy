import csv
import pytest
from pathlib import Path

from flippy.arguments import Arguments
from flippy.mode.training.exercise import Exercise
from flippy.mode.training.mode import TrainingMode
from flippy.othello.board import BLACK, WHITE, Board

OPENINGS_FILE = Path(__file__).parent / "../openings.csv"


@pytest.fixture()
def exercises() -> list[Exercise]:
    args = Arguments.empty()
    openings = TrainingMode(args)
    openings.load_exercises()
    return openings.exercises


def test_sorted() -> None:
    # The openings file should be sorted.
    reader = csv.DictReader(open(OPENINGS_FILE, "r"), delimiter="|")

    stripped_rows: list[dict[str, str]] = []

    for row in reader:
        stripped_row = {k.strip(): v.strip() for (k, v) in row.items()}
        stripped_rows.append(stripped_row)

    def sort_key(row: dict[str, str]) -> tuple[str, str]:
        return (row["color"], row["moves"])

    assert sorted(stripped_rows, key=sort_key) == stripped_rows


def test_all_positions_have_moves(exercises: list[Exercise]) -> None:
    # Every position in the opening file should have at least one move.
    for exercise in exercises:
        for board in exercise.boards:
            assert board.has_moves()


def test_move_sequence_count(exercises: list[Exercise]) -> None:
    # Openings for black should have an odd number of moves, since white starts.
    # Likewise openings for white should have an even number of moves.
    for exercise in exercises:
        if exercise.color == WHITE:
            assert len(exercise.moves) % 2 == 0
        elif exercise.color == BLACK:
            assert len(exercise.moves) % 2 == 1
        else:
            raise NotImplementedError


def test_no_double_transposition_subtree_investigation(
    exercises: list[Exercise],
) -> None:
    # Prevent having multiple transpositions investigate same subtree.

    investigated_subtrees: dict[Board, list[int]] = {}

    for exercise in exercises:
        for moves_done, board in enumerate(exercise.boards):
            if board.turn != exercise.color:
                # Transposition happens user's opponent.
                continue

            if moves_done == len(exercise.moves) - 1:
                # Subtree is not investigated.
                continue

            moves_seq = exercise.moves[:moves_done]

            try:
                investigated_seq = investigated_subtrees[board]
            except KeyError:
                investigated_subtrees[board] = moves_seq
                continue

            if investigated_seq != moves_seq:
                board.show()
                print("Transposition:")
                print("- " + Board.indexes_to_fields(investigated_seq))
                print("- " + Board.indexes_to_fields(moves_seq))

                assert False


def test_uniqueness(exercises: list[Exercise]) -> None:
    # All exercises must be unique.
    exercise_moves: set[tuple[int, ...]] = set()

    for exercise in exercises:
        moves = tuple(exercise.moves)

        if moves in exercise_moves:
            print("non-unique exercise: " + Board.indexes_to_fields(moves))
            assert False

        exercise_moves.add(moves)


def test_consistent_moves(exercises: list[Exercise]) -> None:
    # The same position should always have the same next move, but only when the user is to move.
    next_moves: dict[Board, int] = {}

    for exercise in exercises:
        for moves_done in range(len(exercise.moves)):
            move = exercise.moves[moves_done]
            board = exercise.boards[moves_done]

            if board.turn != exercise.color:
                # User is not to move.
                continue

            try:
                found_move = next_moves[board]
            except KeyError:
                next_moves[board] = move
            else:
                if move != found_move:
                    board.show()
                    print(
                        "Sequence: "
                        + Board.indexes_to_fields(exercise.moves[:moves_done])
                    )
                    print()
                    print("Inconsistent moves:")
                    print("- " + Board.index_to_field(move))
                    print("- " + Board.index_to_field(found_move))
                    assert False


def test_skipped_children(exercises: list[Exercise]) -> None:
    # Rows with skipped children should have certain values

    for exercise in exercises:
        if exercise.has_skipped_children:
            assert exercise.raw["interest"] == "vlow"
            assert exercise.eval is None


def test_tree_exploration(exercises: list[Exercise]) -> None:
    # In every exercise, when the player is to move, the move is either:
    # - not explored
    # - completely explored (all children have an exercise)
    # - partially explored but parent is marked as having skipped children

    board_explored_children: dict[Board, set[int]] = {}
    boards_with_skipped_children: set[Board] = set()
    sequences: dict[Board, list[int]] = {}

    for exercise in exercises:
        if exercise.has_skipped_children:
            boards_with_skipped_children.add(exercise.boards[-1])

        for i in range(len(exercise.moves)):
            move = exercise.moves[i]
            board = exercise.boards[i]

            if exercise.color == board.turn:
                # Opponent not to move.
                continue

            if board not in board_explored_children:
                board_explored_children[board] = set()
                sequences[board] = exercise.moves[:i]

            board_explored_children[board].add(move)

    for board, explored_moves in board_explored_children.items():
        normalized_explored_children = {
            board.do_normalized_move(move) for move in explored_moves
        }

        unexplored_moves = set()
        for move in board.get_moves_as_set():
            child = board.do_normalized_move(move)

            if child not in normalized_explored_children:
                unexplored_moves.add(move)

        has_skipped = board in boards_with_skipped_children

        sequence = Board.indexes_to_fields(sequences[board])

        if has_skipped and not unexplored_moves:
            board.show()
            print("Board is fully explored and has `...` marker.")
            print("Sequence: " + sequence)
            assert False

        if not has_skipped and unexplored_moves:
            board.show()
            print("Board is not fully explored and has no `...` marker.")
            print("Sequence: " + sequence)
            print(
                "Unexplored children: "
                + ", ".join(Board.index_to_field(move) for move in unexplored_moves)
            )
            print(f"Board: {board}")
            print()
            assert False
