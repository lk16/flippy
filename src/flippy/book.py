def get_learn_level(disc_count: int) -> int:
    if disc_count <= 12:
        return 36

    if disc_count <= 20:
        return 34

    return 32
