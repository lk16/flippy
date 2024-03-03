def flip_horizontally(x: int) -> int:
    k1 = 0x5555555555555555
    k2 = 0x3333333333333333
    k4 = 0x0F0F0F0F0F0F0F0F
    x = ((x >> 1) & k1) | ((x & k1) << 1)
    x = ((x >> 2) & k2) | ((x & k2) << 2)
    x = ((x >> 4) & k4) | ((x & k4) << 4)
    return x & 0xFFFFFFFFFFFFFFFF


def flip_vertically(x: int) -> int:
    k1 = 0x00FF00FF00FF00FF
    k2 = 0x0000FFFF0000FFFF

    x = ((x >> 8) & k1) | ((x & k1) << 8)
    x = ((x >> 16) & k2) | ((x & k2) << 16)
    x = (x >> 32) | (x << 32)
    return x & 0xFFFFFFFFFFFFFFFF


def flip_diagonally(x: int) -> int:
    k1 = 0x5500550055005500
    k2 = 0x3333000033330000
    k4 = 0x0F0F0F0F00000000
    t = k4 & (x ^ (x << 28))
    x ^= t ^ (t >> 28)
    t = k2 & (x ^ (x << 14))
    x ^= t ^ (t >> 14)
    t = k1 & (x ^ (x << 7))
    x ^= t ^ (t >> 7)
    return x & 0xFFFFFFFFFFFFFFFF


def bits_rotate(x: int, rotation: int) -> int:
    if rotation & 1:
        x = flip_horizontally(x)
    if rotation & 2:
        x = flip_vertically(x)
    if rotation & 4:
        x = flip_diagonally(x)
    return x


def show_bits(b: int) -> None:
    print("+-a-b-c-d-e-f-g-h-+")
    for y in range(8):
        print("{} ".format(y + 1), end="")
        for x in range(8):
            mask = 1 << ((y * 8) + x)
            if b & mask:
                print("● ", end="")
            else:
                print("  ", end="")
        print("|")
    print("+-----------------+")
