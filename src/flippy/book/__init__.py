# Minimum edax search level for an evaluation to be potentially saved in DB.
MIN_LEARN_LEVEL = 16

# Minimum edax search level when ran from user interface
MIN_UI_SEARCH_LEVEL = 8

# Maximum edax search level when ran from user interface
MAX_UI_SEARCH_LEVEL = 32

# Maxmium number of discs for a board to be potentially saved in DB.
MAX_SAVABLE_DISCS = 30


def get_learn_level(disc_count: int) -> int:
    if disc_count <= 12:
        return 36

    if disc_count <= 20:
        return 34

    return 32
