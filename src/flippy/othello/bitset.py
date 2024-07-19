from __future__ import annotations

from copy import deepcopy

BITSET_MASK = 0xFFFFFFFFFFFFFFFF


class BitSet:
    def __init__(self, value: int) -> None:
        if value & BITSET_MASK != value:
            raise ValueError

        self.__value = value

    def rotated(self, rotation: int) -> BitSet:
        if rotation & 0x7 != rotation:
            raise ValueError

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
        if index not in range(64):
            raise ValueError

        mask = 1 << index
        return self.__value & mask != 0

    def is_set_2d(self, *, x: int, y: int) -> bool:
        if x not in range(8) or y not in range(8):
            raise ValueError

        return self.is_set(8 * y + x)

    def is_empty(self) -> bool:
        return self.__value == 0

    def has_any(self) -> bool:
        return self.__value != 0

    def count_bits(self) -> int:
        return bin(self.__value).count("1")

    def lowest_bit_index(self) -> int:
        if self.is_empty():
            raise ValueError

        return (self.__value & -self.__value).bit_length() - 1

    def show(self) -> None:
        print("+-a-b-c-d-e-f-g-h-+")
        for y in range(8):
            print("{} ".format(y + 1), end="")
            for x in range(8):
                if self.is_set_2d(y=y, x=x):
                    print("● ", end="")
                else:
                    print("  ", end="")
            print("|")
        print("+-----------------+")

    def as_hex(self) -> str:  # pragma: nocover
        return hex(self.__value)

    def as_int(self) -> int:  # pragma: nocover
        return int(self.__value)

    def as_set(self) -> set[int]:
        return {index for index in range(64) if self.is_set(index)}

    def __repr__(self) -> str:  # pragma: nocover
        return f"BitSet({self.as_hex()})"

    def __hash__(self) -> int:  # pragma: nocover
        return hash(self.__value)

    def __eq__(self, rhs: object) -> bool:
        if not isinstance(rhs, BitSet):
            raise TypeError

        return self.__value == rhs.__value

    def __lt__(self, rhs: BitSet) -> bool:
        return self.__value < rhs.__value

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
        if shift not in range(64):
            raise ValueError

        value = (self.__value << shift) & BITSET_MASK
        return BitSet(value)

    def __rshift__(self, shift: int) -> BitSet:
        if shift not in range(64):
            raise ValueError

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
