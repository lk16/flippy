from flippy.edax.types import EdaxEvaluation
from flippy.othello.position import Position

# Minimum edax search level for an evaluation to be potentially saved in DB.
MIN_LEARN_LEVEL = 16

# Minimum edax search level when ran from user interface
MIN_UI_SEARCH_LEVEL = 8

# Maximum edax search level when ran from user interface
MAX_UI_SEARCH_LEVEL = 32

# Maxmium number of discs for a board to be potentially saved in DB.
MAX_SAVABLE_DISCS = 40


def is_savable_evaluation(evaluation: EdaxEvaluation) -> bool:
    """
    Checks whether an evaluation qualifies to be saved in the DB.
    """
    return evaluation.level >= MIN_LEARN_LEVEL and is_savable_position(
        evaluation.position
    )


def is_savable_position(position: Position) -> bool:
    """
    Checks whether a position qualifies to be saved in the DB.
    """
    return position.has_moves() and position.count_discs() <= MAX_SAVABLE_DISCS


def get_learn_level(disc_count: int) -> int:
    if disc_count <= 12:
        return 36

    if disc_count <= 20:
        return 34

    return 32
