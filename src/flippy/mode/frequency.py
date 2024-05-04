from collections import defaultdict
from typing import Any

from flippy.arguments import Arguments
from flippy.config import ALL_USERNAMES, PGN_TARGET_FOLDER
from flippy.mode.game import GameMode
from flippy.othello.game import Game
from flippy.othello.position import Position


class FrequencyMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)
        self.args = args.position_frequency
        self.frequencies = self._load_frequencies()

    def _load_frequencies(self) -> defaultdict[Position, int]:
        frequencies: defaultdict[Position, int] = defaultdict(lambda: 0)
        pgn_files = list((PGN_TARGET_FOLDER / "normal").rglob("*.pgn"))
        usernames = ALL_USERNAMES

        games = [Game.from_pgn(file) for file in pgn_files]

        if self.args.lost_only:
            lost_games: list[Game] = []

            for game in games:
                our_color = game.get_color_any(usernames)
                winner = game.get_winner()

                if our_color is None:
                    # We did not play this game.
                    continue

                if winner is None or winner == our_color:
                    # We won or drew.
                    continue

                lost_games.append(game)

            games = lost_games

        if self.args.most_recent is not None:
            games.sort(key=lambda game: game.get_date(), reverse=True)
            games = games[: self.args.most_recent]

        for game in games:
            for board in game.boards:
                position = board.position.normalized()
                frequencies[position] += 1

        return frequencies

    def get_ui_details(self) -> dict[str, Any]:
        # dict of move to frequency
        child_frequencies: dict[int, int] = {}
        position = self.get_board().position

        for move in range(64):
            if position.is_valid_move(move):
                normalized_child = position.do_normalized_move(move)
                freq = self.frequencies[normalized_child]
                child_frequencies[move] = freq

        return {"child_frequencies": child_frequencies}
