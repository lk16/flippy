import pytest

from flippy.othello.bitset import BITSET_MASK, BitSet

BITSET_EMPTY = BitSet(0x0)

BITSET_LEFT_TOP = BitSet(0x1)
BITSET_RIGHT_TOP = BitSet(0x80)
BITSET_LEFT_BOTTOM = BitSet(0x0100000000000000)
BITSET_RIGHT_BOTTOM = BitSet(0x8000000000000000)


BITSET_SECOND_BIT = BitSet(0x2)

BITSET_TOP_EDGE = BitSet(0xFF)
BITSET_BOTTOM_EDGE = BitSet(0xFF00000000000000)
BITSET_LEFT_EDGE = BitSet(0x0101010101010101)
BITSET_RIGHT_EDGE = BitSet(0x8080808080808080)

BITSET_FULL = BitSet(BITSET_MASK)


@pytest.mark.parametrize(
    ["value"],
    [
        pytest.param(-1, id="too-small"),
        pytest.param(BITSET_MASK + 1, id="too-big"),
    ],
)
def test_init_error(value: int) -> None:
    with pytest.raises(ValueError):
        BitSet(value)


@pytest.mark.parametrize(
    ["rotation"],
    [
        pytest.param(-1, id="too-small"),
        pytest.param(8, id="too-big"),
    ],
)
def test_rotated_error(rotation: int) -> None:
    with pytest.raises(ValueError):
        BITSET_EMPTY.rotated(rotation)


def test_rotated_makes_copy() -> None:
    for rotation in range(8):
        rotated = BITSET_EMPTY.rotated(rotation)
        rotated is not BITSET_EMPTY


@pytest.mark.parametrize(
    ["bit_set"],
    [
        pytest.param(BITSET_EMPTY, id="empty"),
        pytest.param(BITSET_FULL, id="full"),
    ],
)
def test_rotated_unchanging(bit_set: BitSet) -> None:
    for rotation in range(8):
        assert bit_set.rotated(rotation) == bit_set


def test_rotated_corner_bits() -> None:
    expected_bit_sets = [
        BITSET_LEFT_TOP,
        BITSET_RIGHT_TOP,
        BITSET_LEFT_BOTTOM,
        BITSET_RIGHT_BOTTOM,
        BITSET_LEFT_TOP,
        BITSET_LEFT_BOTTOM,
        BITSET_RIGHT_TOP,
        BITSET_RIGHT_BOTTOM,
    ]

    for rotation in range(8):
        rotated = BITSET_LEFT_TOP.rotated(rotation)
        expected = expected_bit_sets[rotation]
        assert expected == rotated


def test_rotated_second_bit() -> None:
    expected_bit_offsets = [1, 6, 57, 62, 8, 48, 15, 55]

    for rotation in range(8):
        rotated = BITSET_SECOND_BIT.rotated(rotation)
        expected = BitSet(1 << expected_bit_offsets[rotation])
        assert expected == rotated


def test_rotated_edge() -> None:
    expected_bit_sets = [
        BITSET_TOP_EDGE,
        BITSET_TOP_EDGE,
        BITSET_BOTTOM_EDGE,
        BITSET_BOTTOM_EDGE,
        BITSET_LEFT_EDGE,
        BITSET_LEFT_EDGE,
        BITSET_RIGHT_EDGE,
        BITSET_RIGHT_EDGE,
    ]

    for rotation in range(8):
        rotated = BITSET_TOP_EDGE.rotated(rotation)
        expected = expected_bit_sets[rotation]
        assert expected == rotated


@pytest.mark.parametrize(
    ["bit_set"],
    [
        pytest.param(BITSET_EMPTY, id="empty"),
        pytest.param(BITSET_LEFT_TOP, id="left-top"),
        pytest.param(BITSET_SECOND_BIT, id="second-bit"),
        pytest.param(BITSET_TOP_EDGE, id="top-edge"),
        pytest.param(BITSET_FULL, id="full"),
    ],
)
def test_unrotated(bit_set: BitSet) -> None:
    for rotation in range(8):
        bit_set.rotated(rotation).unrotated(rotation) == bit_set


def test_is_set_true() -> None:
    assert BITSET_LEFT_TOP.is_set(0)


def test_is_set_false() -> None:
    assert not BITSET_LEFT_TOP.is_set(1)


@pytest.mark.parametrize(
    ["index"],
    [
        pytest.param(-1, id="too-small"),
        pytest.param(64, id="too-big"),
    ],
)
def test_is_set_error(index: int) -> None:
    with pytest.raises(ValueError):
        assert BITSET_LEFT_TOP.is_set(index)


def test_is_set_2d_true() -> None:
    assert BITSET_LEFT_TOP.is_set_2d(y=0, x=0)


def test_is_set_2d_false() -> None:
    assert not BITSET_LEFT_TOP.is_set_2d(y=0, x=1)


@pytest.mark.parametrize(
    ["x", "y"],
    [
        pytest.param(-1, 0, id="x-too-small"),
        pytest.param(8, 0, id="x-too-big"),
        pytest.param(0, -1, id="y-too-small"),
        pytest.param(0, 8, id="y-too-big"),
        pytest.param(-1, 8, id="both-invalid"),
    ],
)
def test_is_set_2d_error(x: int, y: int) -> None:
    with pytest.raises(ValueError):
        assert BITSET_LEFT_TOP.is_set_2d(y=y, x=x)


@pytest.mark.parametrize(
    ["bit_set", "expected"],
    [
        pytest.param(BITSET_EMPTY, True, id="empty"),
        pytest.param(BITSET_LEFT_TOP, False, id="left-top"),
        pytest.param(BITSET_SECOND_BIT, False, id="second-bit"),
        pytest.param(BITSET_TOP_EDGE, False, id="top-edge"),
        pytest.param(BITSET_FULL, False, id="full"),
    ],
)
def test_is_empty(bit_set: BitSet, expected: bool) -> None:
    assert expected == bit_set.is_empty()


@pytest.mark.parametrize(
    ["bit_set", "expected"],
    [
        pytest.param(BITSET_EMPTY, False, id="empty"),
        pytest.param(BITSET_LEFT_TOP, True, id="left-top"),
        pytest.param(BITSET_SECOND_BIT, True, id="second-bit"),
        pytest.param(BITSET_TOP_EDGE, True, id="top-edge"),
        pytest.param(BITSET_FULL, True, id="full"),
    ],
)
def test_has_any(bit_set: BitSet, expected: bool) -> None:
    assert expected == bit_set.has_any()


@pytest.mark.parametrize(
    ["bit_set", "expected"],
    [
        pytest.param(BITSET_EMPTY, 0, id="empty"),
        pytest.param(BITSET_LEFT_TOP, 1, id="left-top"),
        pytest.param(BITSET_SECOND_BIT, 1, id="second-bit"),
        pytest.param(BITSET_TOP_EDGE, 8, id="top-edge"),
        pytest.param(BITSET_FULL, 64, id="full"),
    ],
)
def test_count_bits(bit_set: BitSet, expected: int) -> None:
    assert expected == bit_set.count_bits()


@pytest.mark.parametrize(
    ["bit_set", "expected"],
    [
        pytest.param(BITSET_LEFT_TOP, 0, id="left-top"),
        pytest.param(BITSET_SECOND_BIT, 1, id="second-bit"),
        pytest.param(BITSET_TOP_EDGE, 0, id="top-edge"),
        pytest.param(BITSET_FULL, 0, id="full"),
    ],
)
def test_lowest_bit_index(bit_set: BitSet, expected: int) -> None:
    assert expected == bit_set.lowest_bit_index()


def test_lowest_bit_index_error() -> None:
    with pytest.raises(ValueError):
        BITSET_EMPTY.lowest_bit_index()


def test_show_empty(capsys: pytest.CaptureFixture[str]) -> None:
    BITSET_EMPTY.show()

    printed = capsys.readouterr().out
    expected = (
        "+-a-b-c-d-e-f-g-h-+\n"
        "1                 |\n"
        "2                 |\n"
        "3                 |\n"
        "4                 |\n"
        "5                 |\n"
        "6                 |\n"
        "7                 |\n"
        "8                 |\n"
        "+-----------------+\n"
    )

    assert expected == printed


def test_show_left_top(capsys: pytest.CaptureFixture[str]) -> None:
    BITSET_LEFT_TOP.show()

    printed = capsys.readouterr().out
    expected = (
        "+-a-b-c-d-e-f-g-h-+\n"
        "1 ●               |\n"
        "2                 |\n"
        "3                 |\n"
        "4                 |\n"
        "5                 |\n"
        "6                 |\n"
        "7                 |\n"
        "8                 |\n"
        "+-----------------+\n"
    )

    assert expected == printed


def test_show_second_bit(capsys: pytest.CaptureFixture[str]) -> None:
    BITSET_SECOND_BIT.show()

    printed = capsys.readouterr().out
    expected = (
        "+-a-b-c-d-e-f-g-h-+\n"
        "1   ●             |\n"
        "2                 |\n"
        "3                 |\n"
        "4                 |\n"
        "5                 |\n"
        "6                 |\n"
        "7                 |\n"
        "8                 |\n"
        "+-----------------+\n"
    )

    assert expected == printed


def test_show_top_edge(capsys: pytest.CaptureFixture[str]) -> None:
    BITSET_TOP_EDGE.show()

    printed = capsys.readouterr().out
    expected = (
        "+-a-b-c-d-e-f-g-h-+\n"
        "1 ● ● ● ● ● ● ● ● |\n"
        "2                 |\n"
        "3                 |\n"
        "4                 |\n"
        "5                 |\n"
        "6                 |\n"
        "7                 |\n"
        "8                 |\n"
        "+-----------------+\n"
    )

    assert expected == printed


def test_show_full(capsys: pytest.CaptureFixture[str]) -> None:
    BITSET_FULL.show()

    printed = capsys.readouterr().out
    expected = (
        "+-a-b-c-d-e-f-g-h-+\n"
        "1 ● ● ● ● ● ● ● ● |\n"
        "2 ● ● ● ● ● ● ● ● |\n"
        "3 ● ● ● ● ● ● ● ● |\n"
        "4 ● ● ● ● ● ● ● ● |\n"
        "5 ● ● ● ● ● ● ● ● |\n"
        "6 ● ● ● ● ● ● ● ● |\n"
        "7 ● ● ● ● ● ● ● ● |\n"
        "8 ● ● ● ● ● ● ● ● |\n"
        "+-----------------+\n"
    )

    assert expected == printed


@pytest.mark.parametrize(
    ["bit_set", "expected"],
    [
        pytest.param(BITSET_EMPTY, set(), id="empty"),
        pytest.param(BITSET_LEFT_TOP, {0}, id="left-top"),
        pytest.param(BITSET_SECOND_BIT, {1}, id="second-bit"),
        pytest.param(BITSET_TOP_EDGE, set(range(8)), id="top-edge"),
        pytest.param(BITSET_FULL, set(range(64)), id="full"),
    ],
)
def test_as_set(bit_set: BitSet, expected: set[int]) -> None:
    assert expected == bit_set.as_set()


@pytest.mark.parametrize(
    ["lhs", "rhs", "expected"],
    [
        pytest.param(BITSET_EMPTY, BITSET_EMPTY, True, id="equal"),
        pytest.param(BITSET_EMPTY, BITSET_LEFT_TOP, False, id="not-equal"),
    ],
)
def test_eq_ok(lhs: BitSet, rhs: BitSet, expected: bool) -> None:
    assert (lhs == rhs) == expected


def test_eq_error() -> None:
    with pytest.raises(TypeError):
        assert BITSET_EMPTY == 0


@pytest.mark.parametrize(
    ["bit_set", "expected"],
    [
        pytest.param(BITSET_EMPTY, BITSET_FULL, id="empty"),
        pytest.param(BITSET_TOP_EDGE, BitSet(0xFFFFFFFFFFFFFF00), id="top-edge"),
    ],
)
def test_invert(bit_set: BitSet, expected: BitSet) -> None:
    assert ~bit_set == expected


@pytest.mark.parametrize(
    ["lhs", "rhs", "expected"],
    [
        pytest.param(
            BITSET_LEFT_TOP, BITSET_LEFT_TOP, BITSET_LEFT_TOP, id="bitset-rhs"
        ),
        pytest.param(BITSET_LEFT_TOP, 0, BITSET_EMPTY, id="integer-rhs"),
    ],
)
def test_and(lhs: BitSet, rhs: int | BitSet, expected: BitSet) -> None:
    assert (lhs & rhs) == expected


@pytest.mark.parametrize(
    ["lhs", "rhs", "expected"],
    [
        pytest.param(
            BITSET_LEFT_TOP, BITSET_LEFT_TOP, BITSET_LEFT_TOP, id="bitset-rhs"
        ),
        pytest.param(BITSET_LEFT_TOP, 0, BITSET_LEFT_TOP, id="integer-rhs"),
    ],
)
def test_or(lhs: BitSet, rhs: int | BitSet, expected: BitSet) -> None:
    assert (lhs | rhs) == expected


def test_bool() -> None:
    with pytest.raises(NotImplementedError):
        bool(BITSET_EMPTY)


def test_lshift_ok() -> None:
    assert BITSET_TOP_EDGE << 4 == BitSet(0xFF0)


@pytest.mark.parametrize(
    ["shift"],
    [
        pytest.param(-1, id="too-small"),
        pytest.param(64, id="too-big"),
    ],
)
def test_lshift_error(shift: int) -> None:
    with pytest.raises(ValueError):
        BITSET_EMPTY << shift


def test_rshift_ok() -> None:
    assert BITSET_TOP_EDGE >> 4 == BitSet(0xF)


@pytest.mark.parametrize(
    ["shift"],
    [
        pytest.param(-1, id="too-small"),
        pytest.param(64, id="too-big"),
    ],
)
def test_rshift_error(shift: int) -> None:
    with pytest.raises(ValueError):
        BITSET_EMPTY >> shift
