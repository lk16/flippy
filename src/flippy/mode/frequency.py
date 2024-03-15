from flippy.config import config
from collections import defaultdict
from typing import Any
from flippy.arguments import Arguments
from flippy.mode.game import GameMode
from flippy.othello.board import Board
from flippy.othello.game import Game


class PositionFrequency(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)
        self.args = args.position_frequency
        self.frequencies = self._load_frequencies()

    def _load_frequencies(self) -> defaultdict[Board, int]:
        frequencies: defaultdict[Board, int] = defaultdict(lambda: 0)
        pgn_files = list((config.pgn_target_folder() / "normal").rglob("*.pgn"))
        usernames = config.all_usernames()

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
                normalized, _ = board.normalized()
                frequencies[normalized] += 1

        return frequencies

    def get_ui_details(self) -> dict[str, Any]:
        # dict of move to frequency
        child_frequencies: dict[int, int] = {}
        board = self.get_board()

        for move in range(64):
            if board.is_valid_move(move):
                normalized_child = board.do_normalized_move(move)
                freq = self.frequencies[normalized_child]
                child_frequencies[move] = freq

        return {"child_frequencies": child_frequencies}
