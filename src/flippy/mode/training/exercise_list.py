from flippy.mode.training.exercise import Exercise
from flippy.othello.board import BLACK, WHITE


# fmt: off
def get_exercises() -> list[Exercise]:
    return [
        Exercise(BLACK, "e6 d6 &c5 f4 c6 e7 f6 d7 f5 g4 f7"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 *f6 c6 f5"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 c1 c7"),  # ...
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 d1 e2"),  # ...
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 a5 a4 b5 a6"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 a5 a4 f5 a6"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 b5 c1 a4 a3"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 b5 c1 a5 a4"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 b5 c1 f5 c6"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 f5 c6 a3 c1"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 f5 c6 a4 d1"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 f5 c6 a5 a4"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 f6 c1 b5 d1"),
        Exercise(BLACK, "e6 f4 c3 c4 d3 d6 e3 c2 b3 c5 b4 f3 &d2 e2 b6 f6 c1 c6 f5"),
        Exercise(WHITE, "e6 *f6 f5 d6 c5 e3"),
    ]
