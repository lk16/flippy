import pytest

from flippy.othello.bitset import bits_rotate, bits_unrotate

BITSET_MASK = 0xFFFFFFFFFFFFFFFF

BITSET_EMPTY = 0x0

BITSET_LEFT_TOP = 0x1
BITSET_RIGHT_TOP = 0x80
BITSET_LEFT_BOTTOM = 0x0100000000000000
BITSET_RIGHT_BOTTOM = 0x8000000000000000

BITSET_SECOND_BIT = 0x2

BITSET_TOP_EDGE = 0xFF
BITSET_BOTTOM_EDGE = 0xFF00000000000000
BITSET_LEFT_EDGE = 0x0101010101010101
BITSET_RIGHT_EDGE = 0x8080808080808080

BITSET_FULL = BITSET_MASK


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
        rotated = bits_rotate(BITSET_LEFT_TOP, rotation)
        expected = expected_bit_sets[rotation]
        assert expected == rotated


def test_rotated_second_bit() -> None:
    expected_bit_offsets = [1, 6, 57, 62, 8, 48, 15, 55]

    for rotation in range(8):
        rotated = bits_rotate(BITSET_SECOND_BIT, rotation)
        expected = 1 << expected_bit_offsets[rotation]
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
        rotated = bits_rotate(BITSET_TOP_EDGE, rotation)
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
def test_unrotated(bit_set: int) -> None:
    for rotation in range(8):
        rotated = bits_rotate(bit_set, rotation)
        assert bit_set == bits_unrotate(rotated, rotation)
