from pathlib import Path

from flippy import PROJECT_ROOT
from flippy.config import config
from flippy.othello.game import Game


class RecentGames:
    def __init__(self, count: int) -> None:
        self.count = count
        self.pgn_folder = config.pgn_target_folder() / "normal"
        self.all_usernames = config.all_usernames()

    def find_recent_game_files(self) -> list[Path]:
        all_pgn_files = list(self.pgn_folder.rglob("*.pgn"))
        recent_files = sorted(all_pgn_files, reverse=True)[: self.count]
        return list(reversed(recent_files))

    def get_game_results(self, file: Path) -> list[str]:
        game = Game.from_pgn(file)

        players = [game.get_black_player(), game.get_white_player()]
        color_char = "○"

        if players[1] in self.all_usernames:
            players = list(reversed(players))
            color_char = "●"

        winner = game.get_winning_player()
        score = game.get_black_score()

        if winner is None:
            result = "draw: ◑ + 0"
        elif winner in self.all_usernames:
            result = f"win: {color_char} +{abs(score):>2}"
        else:
            result = f"loss: {color_char} -{abs(score):>2}"

        assert game.file
        path = str(game.file.relative_to(PROJECT_ROOT))

        return players + [result, path]

    def __call__(self) -> None:
        results = [
            self.get_game_results(file) for file in self.find_recent_game_files()
        ]

        col_widths = [0] * len(results[0])

        for row in results:
            for i, col in enumerate(row):
                col_widths[i] = max(len(str(col)), col_widths[i])

        for row in results:
            output_items: list[str] = []
            for col, width in zip(row, col_widths, strict=True):
                output_items.append(f"{col:>{width}}")

            output_line = " ".join(output_items)

            prefix = ""
            postfix = ""
            if "loss" in row[2]:
                prefix = "\x1b[31m"
                postfix = "\x1b[0m"
            if "draw" in row[2]:
                prefix = "\x1b[33m"
                postfix = "\x1b[0m"
            print(prefix + output_line + postfix)
