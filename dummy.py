#!/usr/bin/env python3

import random

from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxRequest
from flippy.othello.position import Position


def new_random_with_discs(n_discs: int) -> Position:
    """Generate a random position with exactly n_discs pieces on the board."""
    assert 4 <= n_discs <= 64, "Number of discs must be between 4 and 64"

    position = Position.start()

    while position.count_discs() < n_discs:
        moves = position.get_moves()
        if moves == 0:
            position = position.pass_move()
            if position.get_moves() == 0:
                # No moves available for either player, start over
                position = Position.start()
            continue

        # Convert moves bitset to list of indices
        valid_moves = list(position.get_moves_as_set())

        # Select random move
        random_move = random.choice(valid_moves)
        position = position.do_move(random_move)

    return position


def main() -> None:
    positions = set()

    while len(positions) < 100:
        for level in range(0, 5):
            discs = 4 + (len(positions) % 61)
            position = new_random_with_discs(discs)
            request = EdaxRequest(positions=[position], level=level, source=None)

            evaluations = start_evaluation_sync(request)

            if not evaluations.values:
                continue  # Something went wrong

            try:
                evaluation = evaluations.values[position.normalized()]
            except KeyError:
                continue  # Something went wrong

            score = evaluation.score
            level = evaluation.level

            print(f"({position.me:#x}, {position.opp:#x}, {level}, {score})")
            positions.add(position)


if __name__ == "__main__":
    main()
