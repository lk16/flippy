import pytest
from typing import Iterable

from flippy.othello.board import BLACK, EMPTY, WHITE, Board
from flippy.othello.position import PASS_MOVE


@pytest.mark.parametrize(
    ["index", "expected"],
    [
        pytest.param(0, "a1", id="index-0"),
        pytest.param(7, "h1", id="index-7"),
        pytest.param(56, "a8", id="index-56"),
        pytest.param(63, "h8", id="index-63"),
        pytest.param(17, "b3", id="index-17"),
    ],
)
def test_index_to_field_ok(index: int, expected: str) -> None:
    assert Board.index_to_field(index) == expected


@pytest.mark.parametrize(
    ["index"],
    [
        pytest.param(-1, id="too-small"),
        pytest.param(64, id="too-big"),
    ],
)
def test_index_to_field_error(index: int) -> None:
    with pytest.raises(ValueError):
        Board.index_to_field(index)


@pytest.mark.parametrize(
    ["indexes", "expected"],
    [
        pytest.param([], "", id="0-indexes"),
        pytest.param([0], "a1", id="1-index"),
        pytest.param([0, 1], "a1 b1", id="2-indexes"),
    ],
)
def test_indexes_to_fields(indexes: Iterable[int], expected: str) -> None:
    assert Board.indexes_to_fields(indexes) == expected


@pytest.mark.parametrize(
    ["field", "expected"],
    [
        pytest.param("a1", 0, id="field-a1"),
        pytest.param("h1", 7, id="field-h1"),
        pytest.param("a8", 56, id="field-a8"),
        pytest.param("h8", 63, id="field-h8"),
        pytest.param("b3", 17, id="field-b3"),
        pytest.param("A1", 0, id="field-A1"),
        pytest.param("H1", 7, id="field-H1"),
        pytest.param("A8", 56, id="field-A8"),
        pytest.param("H8", 63, id="field-H8"),
        pytest.param("B3", 17, id="field-B3"),
        pytest.param("--", PASS_MOVE, id="field---"),
        pytest.param("ps", PASS_MOVE, id="field-ps"),
        pytest.param("PS", PASS_MOVE, id="field-PS"),
        pytest.param("pa", PASS_MOVE, id="field-pa"),
        pytest.param("PA", PASS_MOVE, id="field-PA"),
    ],
)
def test_field_to_index_ok(field: str, expected: int) -> None:
    assert Board.field_to_index(field) == expected


@pytest.mark.parametrize(
    ["field"],
    [
        pytest.param("", id="empty"),
        pytest.param("a", id="to-short"),
        pytest.param("aaa", id="too-long"),
        pytest.param("a9", id="invalid-row"),
        pytest.param("i8", id="invalid-column"),
    ],
)
def test_field_to_index_error(field: str) -> None:
    with pytest.raises(ValueError):
        assert Board.field_to_index(field)


def test_start_board() -> None:
    board = Board.start()
    assert board.turn == BLACK
    # Standard Othello starting position
    assert board.get_square(27) == WHITE
    assert board.get_square(28) == BLACK
    assert board.get_square(35) == BLACK
    assert board.get_square(36) == WHITE
    assert board.count_discs() == 4
    assert board.count_empties() == 60


def test_empty_board() -> None:
    board = Board.empty()
    assert board.turn == BLACK
    assert board.count_discs() == 0
    assert board.count_empties() == 64


@pytest.mark.parametrize(
    ["squares", "turn", "expected_counts"],
    [
        pytest.param([EMPTY] * 64, BLACK, {BLACK: 0, WHITE: 0}, id="empty-board"),
        pytest.param([BLACK] * 64, WHITE, {BLACK: 64, WHITE: 0}, id="all-black"),
        pytest.param([WHITE] * 64, BLACK, {BLACK: 0, WHITE: 64}, id="all-white"),
    ],
)
def test_from_squares(
    squares: list[int], turn: int, expected_counts: dict[int, int]
) -> None:
    board = Board.from_squares(squares, turn)
    assert board.turn == turn
    for color, count in expected_counts.items():
        assert board.count(color) == count


def test_get_square() -> None:
    board = Board.start()
    assert board.get_square(27) == WHITE
    assert board.get_square(28) == BLACK
    assert board.get_square(0) == EMPTY


def test_valid_moves_start_position() -> None:
    board = Board.start()
    valid_moves = board.get_moves_as_set()
    # Standard starting position valid moves
    expected_moves = {19, 26, 37, 44}
    assert valid_moves == expected_moves


def test_do_move() -> None:
    board = Board.start()
    # Make a valid move
    new_board = board.do_move(19)
    assert new_board.turn == WHITE
    assert new_board.get_square(19) == BLACK
    assert new_board.get_square(27) == BLACK  # Flipped piece


def test_pass_move() -> None:
    board = Board.empty()
    new_board = board.pass_move()
    assert new_board.turn == WHITE
    assert new_board.count_discs() == 0


def test_game_end() -> None:
    # Empty board with no valid moves for either player
    board = Board.empty()
    assert board.is_game_end()

    # Starting position is not game end
    board = Board.start()
    assert not board.is_game_end()


@pytest.mark.parametrize(
    ["fields", "expected"],
    [
        pytest.param([], [], id="empty-list"),
        pytest.param(["a1"], [0], id="single-field"),
        pytest.param(["a1", "h8"], [0, 63], id="multiple-fields"),
    ],
)
def test_fields_to_indexes(fields: list[str], expected: list[int]) -> None:
    assert Board.fields_to_indexes(fields) == expected


def test_board_equality() -> None:
    board1 = Board.start()
    board2 = Board.start()
    board3 = Board.empty()

    assert board1 == board2
    assert board1 != board3

    with pytest.raises(TypeError):
        board1 == "not a board"


def test_get_children() -> None:
    board = Board.start()
    children = board.get_children()
    assert len(children) == 4  # Standard starting position has 4 valid moves
    assert all(isinstance(child, Board) for child in children)
    assert all(child.turn == WHITE for child in children)


def test_black_white_getters() -> None:
    # Test with BLACK turn
    board = Board.start()
    assert board.turn == BLACK
    black_bits = board.black()
    white_bits = board.white()
    # Standard starting position has black on 28,35 and white on 27,36
    assert black_bits & (1 << 28) != 0
    assert black_bits & (1 << 35) != 0
    assert white_bits & (1 << 27) != 0
    assert white_bits & (1 << 36) != 0

    # Test with WHITE turn
    board = board.do_move(19)  # Make a move to change turn
    assert board.turn == WHITE
    black_bits = board.black()
    white_bits = board.white()
    assert black_bits & (1 << 28) != 0
    assert black_bits & (1 << 35) != 0
    assert black_bits & (1 << 27) != 0
    assert black_bits & (1 << 19) != 0
    assert white_bits & (1 << 36) != 0
