from typing import Any, Optional

from flippy.mode.training.exercise import Exercise
from flippy.othello.board import BLACK, WHITE

BLACK_TREE: dict[str, Any] = {
    "e6 d6": {  # +7
        "c5 b4": {  # +7
            "b5 b6": {  # +10
                "a5": "",
            },
            "b5 f4": {  # +12
                "c6": "",
            },
            "b5 f6": {  # +7
                "c4": "",
            },
        },
        "c5 b6": {  # +7
            "b5 b4": {  # +10
                "a5": "",  # Transposition
            },
            "b5 f4": {  # +13
                "c6": "",
            },
            "b5 f6": {  # +7
                "c4": "",
            },
        },
        "c5 f4": {  # +8
            "c6 b5": {  # +10
                "c4": "",
            },
            "c6 b6": {  # +13
                "f5": "",
            },
            "c6 b7": {  # +29
                "b6": "",
            },
            "c6 c7": {  # +12
                "d7": "",
            },
            "c6 d7": {  # +9
                "c4": "",
            },
            "c6 e7": {  # +14
                "f6": "",
            },
        },
        "c5 f6": {  # +7
            "c4 b3": {  # +10
                "c6": "",
            },
            "c4 b4": {  # +10
                "f5": "",
            },
            "c4 b5": {  # +7
                "c6": "",
            },
            "c4 c3": {  # +10
                "d3": "",
            },
            "c4 d3": {  # +7
                "c6": "",
            },
            "c4 e3": {  # +7
                "f4": "",
            },
        },
    },
    "e6 f4": {  # +1
        "c3 c4": {  # +1
            "d3 c2": {  # +5
                "f3": "",
            },
            "d3 c6": {  # +8
                "d6": "",
            },
            "d3 d6": {  # +1
                "e3": "",
            },
            "d3 e2": {  # +3
                "d2": "",
            },
            "d3 e7": {  # +11
                "c5": "",
            },
            "d3 f7": {  # +13
                "d6": "",
            },
        },
        "c3 c6": {  # +4
            "c4 b4": {  # +9
                "d3": "",
            },
            "c4 d6": {  # +4
                "f6": "",
            },
            "c4 e7": {  # +6
                "f6": "",
            },
        },
        "c3 d6": {  # +4
            "f6 c4": {  # +4
                "c6": "",
            },
            "f6 c6": {  # +4
                "c4": "",
            },
            "f6 d3": {  # +4
                "c5": "",
            },
            "f6 e7": {  # +5
                "f5": "",
            },
            "f6 g6": {  # +10
                "c7": "",
            },
        },
        "c3 e7": {  # +5
            "f3 b2": {  # +28
                "d3": "",
            },
            "f3 c4": {  # +7
                "f5": "",
            },
            "f3 c5": {  # +8
                "g4": "",
            },
            "f3 e3": {  # +6
                "d3": "",
            },
            "f3 f2": {  # +7
                "f6": "",
            },
        },
    },
    "e6 f6": {  # +2
        "f5 d6": {  # +2
            "c5 b4": {  # +10
                "c4": "",
            },
            "c5 b6": {  # +11
                "e7": "",
            },
            "c5 c4": {  # +5
                "c6": "",
            },
            "c5 e3": {  # +2
                "d3": "",
            },
            "c5 f4": {  # +2
                "d7": "",
            },
            "c5 g4": {  # +4
                "d7": "",
            },
        },
    },
}

WHITE_TREE: dict[str, Any] = {  # ...
}


def get_exercises() -> list[Exercise]:
    def tree_to_exercises(
        color: int, tree: dict[str, Any], prefix: Optional[str] = None
    ) -> list[Exercise]:
        exercises: list[Exercise] = []
        prefix = prefix or ""

        for key, value in tree.items():
            if isinstance(value, dict):
                exercises.extend(tree_to_exercises(color, value, f"{prefix} {key}"))
            else:
                moves = f"{prefix} {key} {value}".strip()
                print(moves)
                exercises.append(Exercise(color, moves))

        return exercises

    return tree_to_exercises(BLACK, BLACK_TREE) + tree_to_exercises(WHITE, WHITE_TREE)
