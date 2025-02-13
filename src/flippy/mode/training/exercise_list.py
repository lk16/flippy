from typing import Any, Optional

from flippy.mode.training.exercise import Exercise
from flippy.othello.board import BLACK, WHITE

BLACK_TREE: dict[str, Any] = {
    "e6": {
        "d6 &c5": {
            "f4 c6": {
                "e7 f6": {
                    "d7 f5": "g4 f7",
                },
            },
        },
        "f4 c3": {
            "c4 d3": {
                "d6 *f6": {
                    "c6 f5": "",
                },
                "d6 e3": {
                    "c2 b3": {
                        "c5 b4": {
                            "f3 &d2": {
                                "c1 c7": "",
                                "d1 e2": "",
                                "e2 b6": {
                                    "a5 a4": {
                                        "b5 a6": "",
                                        "f5 a6": "",
                                    },
                                    "b5 c1": {
                                        "a4 a3": "",
                                        "a5 a4": "",
                                        "f5 c6": "",
                                    },
                                    "f5 c6": {
                                        "a3 c1": "",
                                        "a4 d1": "",
                                        "a5 a4": "",
                                    },
                                    "f6 c1": {
                                        "b5 d1": "",
                                        "c6 f5": "",
                                    },
                                },
                            },
                        }
                    }
                },
            },
        },
    }
}

WHITE_TREE: dict[str, Any] = {
    "e6 *f6": "f5 d6 c5 e3",
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
