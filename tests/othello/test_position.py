import pytest
from typing import Iterable

from flippy.othello.bitset import BitSet
from flippy.othello.position import PASS_MOVE, InvalidMove, Position

POSITION_START = Position.start()
POSITION_EMPTY = Position.empty()
POSITION_AFTER_ONE_MOVE = Position.start().do_move(19)

POSITION_NO_MOVES_WIN = Position(BitSet(0xFFFF), BitSet(0x0))
POSITION_NO_MOVES_DRAW = Position(BitSet(0xFFFF), BitSet(0xFFFF0000))
POSITION_NO_MOVES_LOSS = Position(BitSet(0x0), BitSet(0xFFFF))

POSITION_NEED_TO_PASS = Position(BitSet(0x2), BitSet(0x1))


def test_init_error() -> None:
    with pytest.raises(ValueError):
        Position(BitSet(0x1), BitSet(0x1))


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(
            POSITION_START,
            b"\x00\x00\x00\x10\x08\x00\x00\x00\x00\x00\x00\x08\x10\x00\x00\x00",
            id="start",
        ),
        pytest.param(
            POSITION_EMPTY,
            b"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
            id="empty",
        ),
        pytest.param(
            POSITION_AFTER_ONE_MOVE,
            b"\x00\x00\x00\x00\x10\x00\x00\x00\x00\x00\x08\x18\x08\x00\x00\x00",
            id="after-one-move",
        ),
    ],
)
def test_to_bytes(position: Position, expected: bytes) -> None:
    assert position.to_bytes() == expected


@pytest.mark.parametrize(
    ["position"],
    [
        pytest.param(POSITION_START, id="start"),
        pytest.param(POSITION_EMPTY, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE, id="after-one-move"),
    ],
)
def test_from_bytes(position: Position) -> None:
    bytes_ = position.to_bytes()
    assert Position.from_bytes(bytes_) == position


@pytest.mark.parametrize(
    ["position", "expected_valid_moves"],
    [
        pytest.param(POSITION_START, {19, 26, 37, 44}, id="start"),
        pytest.param(POSITION_EMPTY, {PASS_MOVE}, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE, {18, 20, 34}, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN, {PASS_MOVE}, id="no-moves"),
        pytest.param(POSITION_NEED_TO_PASS, {PASS_MOVE}, id="need-to-pass"),
    ],
)
def test_is_valid_move(position: Position, expected_valid_moves: set[int]) -> None:
    moves = list(range(64)) + [PASS_MOVE]

    for move in moves:
        expected = move in expected_valid_moves
        assert position.is_valid_move(move) == expected


@pytest.mark.parametrize(
    ["position"],
    [
        pytest.param(POSITION_START, id="start"),
        pytest.param(POSITION_EMPTY, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN, id="no-moves"),
        pytest.param(POSITION_NEED_TO_PASS, id="need-to-pass"),
    ],
)
def test_get_moves(position: Position) -> None:
    moves = position.get_moves()

    for index in range(64):
        assert position.is_valid_move(index) == moves.is_set(index)


def test_do_move_pass() -> None:
    position = POSITION_NO_MOVES_LOSS
    child = position.do_move(PASS_MOVE)
    expected = Position(position.opp, position.me)
    assert child == expected


def test_do_move_square_taken() -> None:
    with pytest.raises(InvalidMove):
        POSITION_NO_MOVES_LOSS.do_move(0)


def test_do_move_no_flips() -> None:
    with pytest.raises(InvalidMove):
        POSITION_NO_MOVES_LOSS.do_move(63)


def test_do_move_ok() -> None:
    child = POSITION_AFTER_ONE_MOVE.do_move(18)
    expected = Position(BitSet(0x810080000), BitSet(0x1008040000))
    assert child == expected


@pytest.mark.parametrize(
    ["position"],
    [
        pytest.param(POSITION_START, id="start"),
        pytest.param(POSITION_EMPTY, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN, id="no-moves"),
        pytest.param(POSITION_NEED_TO_PASS, id="need-to-pass"),
    ],
)
def test_rotated(position: Position) -> None:
    for rotation in range(8):
        assert position.rotated(rotation).unrotated(rotation) == position


@pytest.mark.parametrize(
    ["position"],
    [
        pytest.param(POSITION_START, id="start"),
        pytest.param(POSITION_EMPTY, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN, id="no-moves"),
        pytest.param(POSITION_NEED_TO_PASS, id="need-to-pass"),
    ],
)
def test_normalize(position: Position) -> None:
    normalized, rotation = position.normalize()
    assert normalized.unrotated(rotation) == position


@pytest.mark.parametrize(
    ["position"],
    [
        pytest.param(POSITION_START, id="start"),
        pytest.param(POSITION_EMPTY, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN, id="no-moves"),
        pytest.param(POSITION_NEED_TO_PASS, id="need-to-pass"),
    ],
)
def test_normalized(position: Position) -> None:
    normalized = position.normalized()

    for rotation in range(8):
        assert position.rotated(rotation).normalized() == normalized


def test_pass_move() -> None:
    position = POSITION_START
    child = position.pass_move()
    assert child == Position(position.opp, position.me)
    assert child.me is not position.opp
    assert child.opp is not position.me


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(POSITION_START, False, id="start"),
        pytest.param(POSITION_EMPTY, True, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE, False, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN, True, id="no-moves"),
        pytest.param(POSITION_NEED_TO_PASS, False, id="need-to-pass"),
    ],
)
def test_is_game_end(position: Position, expected: bool) -> None:
    assert position.is_game_end() == expected


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(POSITION_START, 0, id="start"),
        pytest.param(POSITION_EMPTY, 0, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE, -62, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN, 64, id="no-moves-win"),
        pytest.param(POSITION_NO_MOVES_DRAW, 0, id="no-moves-draw"),
        pytest.param(POSITION_NO_MOVES_LOSS, -64, id="no-moves-loss"),
        pytest.param(POSITION_NEED_TO_PASS, 0, id="need-to-pass"),
    ],
)
def test_get_final_score(position: Position, expected: int) -> None:
    assert position.get_final_score() == expected


def test_show_start(capsys: pytest.CaptureFixture[str]) -> None:
    POSITION_START.show()

    printed = capsys.readouterr().out
    expected = (
        "+-a-b-c-d-e-f-g-h-+\n"
        "1                 |\n"
        "2                 |\n"
        "3       ·         |\n"
        "4     · ● ○       |\n"
        "5       ○ ● ·     |\n"
        "6         ·       |\n"
        "7                 |\n"
        "8                 |\n"
        "+-----------------+\n"
    )

    assert expected == printed


def test_show_after_one_move(capsys: pytest.CaptureFixture[str]) -> None:
    POSITION_AFTER_ONE_MOVE.show()

    printed = capsys.readouterr().out
    expected = (
        "+-a-b-c-d-e-f-g-h-+\n"
        "1                 |\n"
        "2                 |\n"
        "3     · ● ·       |\n"
        "4       ● ●       |\n"
        "5     · ● ○       |\n"
        "6                 |\n"
        "7                 |\n"
        "8                 |\n"
        "+-----------------+\n"
    )

    assert expected == printed


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
    assert Position.index_to_field(index) == expected


@pytest.mark.parametrize(
    ["index"],
    [
        pytest.param(-1, id="too-small"),
        pytest.param(64, id="too-big"),
    ],
)
def test_index_to_field_error(index: int) -> None:
    with pytest.raises(ValueError):
        Position.index_to_field(index)


@pytest.mark.parametrize(
    ["indexes", "expected"],
    [
        pytest.param([], "", id="0-indexes"),
        pytest.param([0], "a1", id="1-index"),
        pytest.param([0, 1], "a1 b1", id="2-indexes"),
    ],
)
def test_indexes_to_fields(indexes: Iterable[int], expected: str) -> None:
    assert Position.indexes_to_fields(indexes) == expected


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
    ],
)
def test_field_to_index_ok(field: str, expected: int) -> None:
    assert Position.field_to_index(field) == expected


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
        assert Position.field_to_index(field)


def test_rotate_move_normal() -> None:
    move = 2
    expected_rotated_moves = [2, 5, 58, 61, 16, 40, 23, 47]

    for rotation in range(8):
        expected = expected_rotated_moves[rotation]
        assert Position.rotate_move(move, rotation) == expected


def test_rotate_move_pass() -> None:
    move = PASS_MOVE

    for rotation in range(8):
        assert Position.rotate_move(move, rotation) == PASS_MOVE


@pytest.mark.parametrize(
    ["move"],
    [
        pytest.param(2, id="normal"),
        pytest.param(PASS_MOVE, id="pass"),
    ],
)
def test_unrotate(move: int) -> None:
    for rotation in range(8):
        rotated_move = Position.rotate_move(move, rotation)
        assert Position.unrotate_move(rotated_move, rotation) == move


@pytest.mark.parametrize(
    ["lhs", "rhs", "expected"],
    [
        pytest.param(POSITION_EMPTY, POSITION_EMPTY, True, id="equal"),
        pytest.param(POSITION_EMPTY, POSITION_START, False, id="not-equal"),
    ],
)
def test_eq_ok(lhs: Position, rhs: Position, expected: bool) -> None:
    assert (lhs == rhs) == expected


def test_eq_error() -> None:
    with pytest.raises(TypeError):
        POSITION_EMPTY == 0


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(
            POSITION_EMPTY,
            "---------------------------------------------------------------- X;\n",
            id="empty",
        ),
        pytest.param(
            POSITION_START,
            "---------------------------OX------XO--------------------------- X;\n",
            id="start",
        ),
        pytest.param(
            POSITION_AFTER_ONE_MOVE,
            "-------------------O-------OO------OX--------------------------- X;\n",
            id="after-one-move",
        ),
    ],
)
def test_to_problem(position: Position, expected: str) -> None:
    assert position.to_problem() == expected


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(POSITION_EMPTY, 0, id="empty"),
        pytest.param(POSITION_START, 4, id="start"),
        pytest.param(POSITION_AFTER_ONE_MOVE, 5, id="after-one-move"),
    ],
)
def test_count_discs(position: Position, expected: int) -> None:
    assert position.count_discs() == expected


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(POSITION_EMPTY, 64, id="empty"),
        pytest.param(POSITION_START, 60, id="start"),
        pytest.param(POSITION_AFTER_ONE_MOVE, 59, id="after-one-move"),
    ],
)
def test_count_empties(position: Position, expected: int) -> None:
    assert position.count_empties() == expected
