from __future__ import annotations

from copy import deepcopy

BITSET_MASK = 0xFFFFFFFFFFFFFFFF


class BitSet:
    def __init__(self, value: int) -> None:
        assert value & BITSET_MASK == value

        self.__value = value

    def rotated(self, rotation: int) -> BitSet:
        assert rotation & 0x7 == rotation

        bit_set = deepcopy(self)

        if rotation & 0x1:
            bit_set = bit_set.__flip_horizontally()

        if rotation & 0x2:
            bit_set = bit_set.__flip_vertically()

        if rotation & 0x4:
            bit_set = bit_set.__flip_diagonally()

        return bit_set

    def unrotated(self, rotation: int) -> BitSet:
        reverse_rotation = [0, 1, 2, 3, 4, 6, 5, 7]
        return self.rotated(reverse_rotation[rotation])

    def is_set(self, index: int) -> bool:
        assert index in range(64)
        return self.__value & (1 << index) != 0

    def is_set_2d(self, x: int, y: int) -> bool:
        assert x in range(8)
        assert y in range(8)
        return self.is_set(8 * y + x)

    def is_empty(self) -> bool:
        return self.__value == 0

    def has_any(self) -> bool:
        return self.__value != 0

    def count(self) -> int:
        return bin(self.__value).count("1")

    def lowest_bit_index(self) -> int:
        return (self.__value & -self.__value).bit_length() - 1

    def show(self) -> None:
        print("+-a-b-c-d-e-f-g-h-+")
        for y in range(8):
            print("{} ".format(y + 1), end="")
            for x in range(8):
                if self.is_set_2d(x, y):
                    print("● ", end="")
                else:
                    print("  ", end="")
            print("|")
        print("+-----------------+")

    def as_hex(self) -> str:
        return hex(self.__value)

    def __repr__(self) -> str:
        return f"BitSet({self.as_hex()})"

    def __hash__(self) -> int:
        return hash(self.__value)

    def __invert__(self) -> BitSet:
        return BitSet(~self.__value & BITSET_MASK)

    def __and__(self, rhs: int | BitSet) -> BitSet:
        if isinstance(rhs, int):
            rhs = BitSet(rhs)

        return BitSet(self.__value & rhs.__value)

    def __or__(self, rhs: int | BitSet) -> BitSet:
        if isinstance(rhs, int):
            rhs = BitSet(rhs)

        return BitSet(self.__value | rhs.__value)

    def __bool__(self) -> bool:
        raise NotImplementedError("Use the more explicit `BitSet.has_any()` instead.")

    def __lshift__(self, shift: int) -> BitSet:
        assert shift in range(64)
        value = (self.__value << shift) & BITSET_MASK
        return BitSet(value)

    def __rshift__(self, shift: int) -> BitSet:
        assert shift in range(64)
        value = (self.__value >> shift) & BITSET_MASK
        return BitSet(value)

    def __flip_horizontally(self) -> BitSet:
        k1 = 0x5555555555555555
        k2 = 0x3333333333333333
        k4 = 0x0F0F0F0F0F0F0F0F

        x = self.__value
        x = ((x >> 1) & k1) | ((x & k1) << 1)
        x = ((x >> 2) & k2) | ((x & k2) << 2)
        x = ((x >> 4) & k4) | ((x & k4) << 4)
        return BitSet(x & BITSET_MASK)

    def __flip_vertically(self) -> BitSet:
        k1 = 0x00FF00FF00FF00FF
        k2 = 0x0000FFFF0000FFFF

        x = self.__value
        x = ((x >> 8) & k1) | ((x & k1) << 8)
        x = ((x >> 16) & k2) | ((x & k2) << 16)
        x = (x >> 32) | (x << 32)
        return BitSet(x & BITSET_MASK)

    def __flip_diagonally(self) -> BitSet:
        k1 = 0x5500550055005500
        k2 = 0x3333000033330000
        k4 = 0x0F0F0F0F00000000

        x = self.__value
        t = k4 & (x ^ (x << 28))
        x ^= t ^ (t >> 28)
        t = k2 & (x ^ (x << 14))
        x ^= t ^ (t >> 14)
        t = k1 & (x ^ (x << 7))
        x ^= t ^ (t >> 7)
        return BitSet(x & BITSET_MASK)
