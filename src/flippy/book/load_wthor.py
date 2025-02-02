from pathlib import Path

from flippy.book.load_pgn import learn_new_positions
from flippy.othello.game import Game
from flippy.othello.position import NormalizedPosition
from flippy.othello.wthor import Wthor


def load_wthor(filenames: list[Path]) -> None:
    positions: set[NormalizedPosition] = set()
    games: list[Game] = []

    print("Loading files.")

    for i, filename in enumerate(filenames):
        games += Wthor(filename).get_games()
        print(f"Loaded {i + 1}/{len(filenames)} files.")

    for i, game in enumerate(games):
        positions.update(game.get_normalized_positions())

        if (i + 1) % 100 == 0 or i == len(games) - 1:
            print(
                f"Loaded game {i + 1}/{len(games)} | {len(positions)} unique positions."
            )

    print(f"Found {len(games)} games with {len(positions)} unique positions.")

    learn_new_positions(positions)
