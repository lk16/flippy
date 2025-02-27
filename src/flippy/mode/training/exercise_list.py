from typing import Any, Optional

from flippy.mode.training.exercise import Exercise
from flippy.othello.board import BLACK, WHITE

BLACK_TREE: dict[str, Any] = {
    "e6": {
        "d6 c5": {  # +7
            "b4 b5": [  # +7
                "b6 a5",  # +10
                "f4 c6",  # +12
                "f6 c4",  # +7
            ],
            "b6 b5": [  # +7
                "b4 a5",  # +10, Transposition
                "f4 c6",  # +13
                "f6 c4",  # +7
            ],
            "f4 c6": [  # +8
                "b5 c4",  # +10
                "b6 f5",  # +13
                "b7 b6",  # +29
                "c7 d7",  # +9
                "d7 c4",  # +14
                "e7 f6",  # +14
            ],
            "f6 c4": [  # +7
                "b3 c6",  # +10
                "b4 f5",  # +10
                "b5 c6",  # +7
                "c3 d3",  # +10
                "d3 c6",  # +7
                "e3 f4",  # +7
            ],
        },
        "f4 c3": {  # +1
            "c4 d3": [  # +1
                "c2 f3",  # +5
                "c6 d6",  # +8
                "d6 e3",  # +1
                "e2 d2",  # +3
                "e7 c5",  # +11
                "f7 d6",  # +13
            ],
            "c6 c4": [  # +4
                "b4 d3",  # +9
                "d6 f6",  # +4
                "e7 f6",  # +6
            ],
            "d6 f6": [  # +4
                "c4 c6",  # +4
                "c6 c4",  # +4
                "d3 c5",  # +4
                "e7 f5",  # +5
                "g6 c7",  # +10
            ],
            "e7 f3": [  # +5
                "b2 d3",  # +28
                "c4 f5",  # +7
                "c5 g4",  # +8
                "e3 d3",  # +6
                "f2 f6",  # +7
            ],
        },
        "f6 f5": {  # +2
            "d6 c5": [  # +2
                "b4 c4",  # +10
                "b6 e7",  # +11
                "c4 c6",  # +5
                "e3 d3",  # +2
                "f4 d7",  # +2
                "g4 d7",  # +4
            ],
            "f4 transposition": "",
        },
    },
}

WHITE_TREE: dict[str, Any] = {
    "e6 f4": {
        "c3 c4": [  # +2
            "b3 d6",  # +10
            "c5 c6",  # +15
            "d3 d6",  # +2
            "e3 d6",  # +4
            "f3 d6",  # +5
            "g3 c6",  # +13
        ],
        "d3 c4": "",  # +2
        "e3 d6": "",  # +2
        "f3 d6": "",  # +4
        "g3 c6": "",  # +10
    },
    "c4 transposition": "",
    "d3 transposition": "",
    "f5 transposition": "",
}


def get_exercises() -> list[Exercise]:
    def tree_to_exercises(
        color: int, tree: dict[str, Any], prefix: Optional[str] = None
    ) -> list[Exercise]:
        exercises: list[Exercise] = []
        prefix = prefix or ""

        for key, value in tree.items():
            if "transposition" in key:
                continue

            if isinstance(value, dict):
                exercises.extend(tree_to_exercises(color, value, f"{prefix} {key}"))

            elif isinstance(value, list):
                for value_item in value:
                    moves = f"{prefix} {key} {value_item}".strip()
                    print(moves)
                    exercises.append(Exercise(color, moves))

            elif isinstance(value, str):
                if "transposition" in value:
                    continue

                moves = f"{prefix} {key} {value}".strip()
                print(moves)
                exercises.append(Exercise(color, moves))
            else:
                raise ValueError(f"Unknown value type: {type(value)}")

        return exercises

    return tree_to_exercises(BLACK, BLACK_TREE) + tree_to_exercises(WHITE, WHITE_TREE)
