import json
from pathlib import Path
from flippy.board import BLACK, WHITE, Board

from flippy.openings_training import OpeningsTraining


OPENINGS_FILE = Path(__file__).parent / "../openings.json"


def test_sorted() -> None:
    # The openings file should be sorted.
    openings = json.loads(OPENINGS_FILE.read_text())
    assert sorted(openings) == openings


def test_valid_moves() -> None:
    # The opening file should only have sequences of valid moves.
    OpeningsTraining(OPENINGS_FILE)


def test_all_positions_have_moves() -> None:
    # Every position in the opening file should have at least one move.
    openings = OpeningsTraining(OPENINGS_FILE)
    for exercise in openings.remaining_exercises:
        for board in exercise.boards:
            assert board.has_moves()


def test_move_sequence_count() -> None:
    # Openings for black should have an odd number of moves, since white starts.
    # Likewise openings for white should have an even number of moves.
    openings = OpeningsTraining(OPENINGS_FILE)
    for exercise in openings.remaining_exercises:
        if exercise.color == WHITE:
            assert len(exercise.moves) % 2 == 0
        elif exercise.color == BLACK:
            assert len(exercise.moves) % 2 == 1
        else:
            raise NotImplementedError


def test_score_estimation() -> None:
    # Every opening should have an even score estimation or a "?" in the third column.
    openings = OpeningsTraining(OPENINGS_FILE)
    for exercise in openings.remaining_exercises:
        score = exercise.raw_input[2]
        try:
            int(score)
        except ValueError:
            assert score == "?"
        else:
            assert int(score) % 2 == 0


def test_no_double_transposition_subtree_investigation() -> None:
    # Prevent having multiple transpositions investigate same subtree.

    investigated_subtrees: dict[Board, list[int]] = {}

    openings = OpeningsTraining(OPENINGS_FILE)
    for exercise in openings.remaining_exercises:
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
                print("- " + Board.offsets_to_str(investigated_seq))
                print("- " + Board.offsets_to_str(moves_seq))

                assert False


def test_notes_column() -> None:
    ALLOWED_PREFIXES = ["boring", "weird", "interesting", "transposition"]

    openings = OpeningsTraining(OPENINGS_FILE)
    for exercise in openings.remaining_exercises:
        assert any(
            exercise.raw_input[3].startswith(prefix) for prefix in ALLOWED_PREFIXES
        )


def test_uniqueness() -> None:
    # All exercises must be unique.
    exercise_moves: set[tuple[int, ...]] = set()

    openings = OpeningsTraining(OPENINGS_FILE)
    for exercise in openings.remaining_exercises:
        moves = tuple(exercise.moves)

        if moves in exercise_moves:
            print("non-unique exercise: " + Board.offsets_to_str(moves))
            assert False

        exercise_moves.add(moves)


def test_consistent_moves() -> None:
    # The same position should always have the same next move, but only when the user is to move.
    next_moves: dict[Board, int] = {}

    openings = OpeningsTraining(OPENINGS_FILE)
    for exercise in openings.remaining_exercises:
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
                        "Sequence: " + Board.offsets_to_str(exercise.moves[:moves_done])
                    )
                    print()
                    print("Inconsistent moves:")
                    print("- " + Board.offset_to_str(move))
                    print("- " + Board.offset_to_str(found_move))
                    assert False
