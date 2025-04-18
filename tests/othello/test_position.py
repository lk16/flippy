import pytest

from flippy.othello.position import PASS_MOVE, InvalidMove, NormalizedPosition, Position

POSITION_START = Position.start()
POSITION_EMPTY = Position.empty()
POSITION_AFTER_ONE_MOVE = Position.start().do_move(19)

POSITION_NO_MOVES_WIN = Position(0xFFFF, 0x0)
POSITION_NO_MOVES_DRAW = Position(0xFFFF, 0xFFFF0000)
POSITION_NO_MOVES_LOSS = Position(0x0, 0xFFFF)

POSITION_NEED_TO_PASS = Position(0x2, 0x1)


def test_init_error() -> None:
    with pytest.raises(ValueError):
        Position(0x1, 0x1)


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
        is_valid = moves & (1 << index) != 0
        assert position.is_valid_move(index) == is_valid


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
    expected = Position(0x810080000, 0x1008040000)
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
    assert normalized.to_position().unrotated(rotation) == position


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


def test_unrotate_move_normal() -> None:
    move = 2
    expected_rotated_moves = [2, 5, 58, 61, 16, 23, 40, 47]

    for rotation in range(8):
        expected = expected_rotated_moves[rotation]
        assert Position.unrotate_move(move, rotation) == expected


def test_unrotate_move_pass() -> None:
    move = PASS_MOVE

    for rotation in range(8):
        assert Position.unrotate_move(move, rotation) == PASS_MOVE


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
            "---------------------------XO------OOO-------------------------- X;\n",
            id="after-one-move",
        ),
    ],
)
def test_to_problem(position: Position, expected: str) -> None:
    normalized = position.normalized()
    assert normalized.to_problem() == expected


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
def test_get_normalized_children(position: Position) -> None:
    # Get normalized children using the method under test
    normalized_children = position.get_normalized_children()

    # Get children the long way and normalize them
    regular_children = position.get_children()
    normalized_regular = {child.normalized() for child in regular_children}

    # Both sets should be equal
    assert normalized_children == normalized_regular


def test_normalized_position_to_from_position_roundtrip() -> None:
    """Test that converting to Position and back preserves the normalized position"""
    original = POSITION_START.normalized()
    position = original.to_position()
    converted = NormalizedPosition(position)
    assert original == converted


def test_normalized_position_to_from_api_roundtrip() -> None:
    """Test that converting to API string and back preserves the normalized position"""
    original = POSITION_START.normalized()
    api_str = original.to_api()
    converted = NormalizedPosition.from_api(api_str)
    assert original == converted
    assert len(api_str) == 32  # Verify string length


def test_normalized_position_to_from_bytes_roundtrip() -> None:
    """Test that converting to bytes and back preserves the normalized position"""
    original = POSITION_START.normalized()
    bytes_ = original.to_bytes()
    converted = NormalizedPosition.from_bytes(bytes_)
    assert original == converted
    assert len(bytes_) == 16  # 2 uint64 = 16 bytes


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(POSITION_START.normalized(), False, id="start"),
        pytest.param(POSITION_EMPTY.normalized(), True, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE.normalized(), False, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN.normalized(), True, id="no-moves"),
        pytest.param(POSITION_NEED_TO_PASS.normalized(), False, id="need-to-pass"),
    ],
)
def test_normalized_position_is_game_end(
    position: NormalizedPosition, expected: bool
) -> None:
    assert position.is_game_end() == expected


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(POSITION_START.normalized(), True, id="start"),
        pytest.param(POSITION_EMPTY.normalized(), False, id="empty"),
        pytest.param(POSITION_AFTER_ONE_MOVE.normalized(), True, id="after-one-move"),
        pytest.param(POSITION_NO_MOVES_WIN.normalized(), False, id="no-moves"),
        pytest.param(POSITION_NEED_TO_PASS.normalized(), False, id="need-to-pass"),
    ],
)
def test_normalized_position_has_moves(
    position: NormalizedPosition, expected: bool
) -> None:
    assert position.has_moves() == expected


def test_normalized_position_pass_move() -> None:
    """Test that passing a move returns a normalized position with players swapped"""
    position = POSITION_START.normalized()
    passed = position.pass_move()
    # Verify the passed position is normalized and players are swapped
    assert passed == Position(POSITION_START.opp, POSITION_START.me).normalized()


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(POSITION_START.normalized(), True, id="start-savable"),
        pytest.param(POSITION_EMPTY.normalized(), False, id="empty-not-savable"),
        pytest.param(
            POSITION_NO_MOVES_WIN.normalized(), False, id="no-moves-not-savable"
        ),
    ],
)
def test_normalized_position_is_db_savable(
    position: NormalizedPosition, expected: bool
) -> None:
    assert position.is_db_savable() == expected


@pytest.mark.parametrize(
    ["position", "expected"],
    [
        pytest.param(
            POSITION_EMPTY.normalized(),
            "---------------------------------------------------------------- X;\n",
            id="empty",
        ),
        pytest.param(
            POSITION_START.normalized(),
            "---------------------------OX------XO--------------------------- X;\n",
            id="start",
        ),
        pytest.param(
            POSITION_AFTER_ONE_MOVE.normalized(),
            "---------------------------XO------OOO-------------------------- X;\n",
            id="after-one-move",
        ),
    ],
)
def test_normalized_position_to_problem(
    position: NormalizedPosition, expected: str
) -> None:
    assert position.to_problem() == expected


@pytest.mark.parametrize(
    "disc_count",
    [
        pytest.param(3, id="too_few_discs"),
        pytest.param(65, id="too_many_discs"),
    ],
)
def test_random_invalid_disc_count(disc_count: int) -> None:
    with pytest.raises(
        ValueError,
        match="Cannot create random position with less than 4 or more than 64 discs",
    ):
        Position.random(disc_count)


@pytest.mark.parametrize(
    "disc_count",
    [
        pytest.param(4, id="minimum_discs"),
        pytest.param(20, id="mid_game"),
        pytest.param(64, id="full_board"),
    ],
)
def test_random_valid_disc_count(disc_count: int) -> None:
    # Test multiple times since it's random
    for _ in range(5):
        position = Position.random(disc_count)

        # Check disc count is correct
        assert position.count_discs() == disc_count

        # Verify no overlapping discs
        assert position.me & position.opp == 0
