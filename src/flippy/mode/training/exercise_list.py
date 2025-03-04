from __future__ import annotations

from typing import Optional

from flippy.mode.training.exercise import Exercise
from flippy.othello.board import BLACK, WHITE


class Node:
    def __init__(
        self,
        moves: str,
        *,
        e: Optional[int] = None,
        c: Optional[list[Node]] = None,
        t: Optional[str] = None,
    ):
        self.moves = moves
        self.eval = e
        self.children = c or []
        self.transposition = t


# fmt:off
BLACK_TREE = Node("e6", c=[
    Node("d6 c5", e=7, c=[
        Node("b4 b5", e=7, c=[
            Node("b6 a5", e=10),
            Node("f4 c6", e=12),
            Node("f6 c4", e=7),
        ]),
        Node("b6 b5", e=7, c=[
            Node("b4 a5", e=10, t='e6 d6 c5 b4 b5 b6 a5'),
            Node("f4 c6", e=13),
            Node("f6 c4", e=7),
        ]),
        Node("f4 c6", e=8, c=[
            Node("b5 c4", e=10),
            Node("b6 f5", e=13),
            Node("b7 b6", e=29),
            Node("c7 d7", e=9),
            Node("d7 c4", e=14),
            Node("e7 f6", e=14),
        ]),
        Node("f6 c4", e=7, c=[
            Node("b3 c6", e=10),
            Node("b4 f5", e=10),
            Node("b5 c6", e=7),
            Node("c3 d3", e=10),
            Node("d3 c6", e=7),
            Node("e3 f4", e=7),
        ]),
    ]),
    Node("f4 c3", e=1, c=[
        Node("c4 d3", e=1, c=[
            Node("c2 f3", e=5),
            Node("c6 d6", e=8),
            Node("d6 e3", e=1),
            Node("e2 d2", e=3),
            Node("e7 c5", e=11),
            Node("f7 d6", e=13),
        ]),
        Node("c6 c4", e=4, c=[
            Node("b4 d3", e=9),
            Node("d6 f6", e=4),
            Node("e7 f6", e=6),
        ]),
        Node("d6 f6", e=4, c=[
            Node("c4 c6", e=4),
            Node("c6 c4", e=4, t='e6 f4 c3 c6 c4 d6 f6'),
            Node("d3 c5", e=4),
            Node("e7 f5", e=5),
            Node("g6 c7", e=10),
        ]),
        Node("e7 f3", e=5, c=[
            Node("b2 d3", e=28),
            Node("c4 f5", e=7),
            Node("c5 g4", e=8),
            Node("e3 d3", e=6),
            Node("f2 f6", e=7),
        ]),
    ]),
    Node("f6 f5", e=2, c=[
        Node("d6 c5", e=2, c=[
            Node("b4 c4", e=10, t='e6 d6 c5 f6 c4 b4 f5'),
            Node("b6 e7", e=11),
            Node("c4 c6", e=5),
            Node("e3 d3", e=2),
            Node("f4 d7", e=2),
            Node("g4 d7", e=4),
        ]),
        Node("f4 e3", t='e6 f6 f5 d6 c5'),
    ]),
])

WHITE_TREE = Node("", c=[
    Node("e6 f4", c=[
        Node("c3 c4", c=[
            Node("b3 d6", e=10),
            Node("c5 c6", e=15),
            Node("d3 d6", e=2),
            Node("e3 d6", e=4),
            Node("f3 d6", e=5),
            Node("g3 c6", e=13),
        ]),
        Node("d3 c4", c=[
            Node("b3 e7", e=10),
            Node("b5 e7", e=10),
            Node("c3 d6", e=2, t='e6 f4 c3 c4 d3 d6'),
            Node("e3 d6", e=5),
            Node("f3 d6", e=7),
            Node("f5 d6", e=4),
            Node("g3 c6", e=6),
        ]),
        Node("e3 d6", c=[
            Node("c4 c5", e=4),
            Node("c5 f3", e=2),
            Node("c6 c5", e=5),
            Node("g4 f6", e=7),
            Node("g5 f6", e=8),
        ]),
        Node("f3 d6", c=[
            Node("c4 d3", e=8),
            Node("c6 e3", e=11),
            Node("f5 f7", e=4),
            Node("g4 e7", e=12),
        ]),
        Node("g3 c6", c=[
            Node("c3 c4", e=13, t='e6 f4 c3 c4 g3 c6'),
            Node("c4 f3", e=11),
            Node("c5 d6", e=12),
            Node("e3 f6", e=12),
        ]),
    ]),
    Node("c4 e3", t='e6 f4'),
    Node("d3 c5", t='e6 f4'),
    Node("f5 d6", t='e6 f4'),
])
# fmt:on


def get_exercises() -> list[Exercise]:
    def tree_to_exercises(color: int, tree: Node, prefix: str) -> list[Exercise]:
        exercises: list[Exercise] = []

        for child in tree.children:
            if child.transposition is not None:
                continue

            if child.children:
                exercises.extend(
                    tree_to_exercises(color, child, f"{prefix} {tree.moves}")
                )

            else:
                moves = f"{prefix} {tree.moves} {child.moves}".strip()
                print(moves)
                exercises.append(Exercise(color, moves))

        return exercises

    return tree_to_exercises(BLACK, BLACK_TREE, "") + tree_to_exercises(
        WHITE, WHITE_TREE, ""
    )
