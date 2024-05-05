import os
import requests
import typer
from datetime import datetime
from itertools import count
from pathlib import Path

from flippy.config import config
from flippy.othello.game import Game


class PgnOrganizer:
    def __init__(self) -> None:
        self.source_folders = config.pgn_source_folders()
        self.target_folder = config.pgn_target_folder()
        self.playok_usernames = config.playok_usernames()

    def __call__(self) -> None:
        moved = self.move_from_source_folders()

        if moved != 0:
            print(f"Moved {moved} PGN files to {self.target_folder}")

        newly_downloaded = self.download_from_playok()

        if newly_downloaded != 0:
            print(f"Downloaded {newly_downloaded} PGN files from playok")

    def move_from_source_folders(self) -> int:
        pgn_files: list[tuple[Path, datetime]] = []

        for source_folder in self.source_folders:
            for pgn_file in source_folder.rglob("*.pgn"):
                created_unix = os.stat(pgn_file).st_ctime
                created = datetime.fromtimestamp(created_unix)
                pgn_files.append((pgn_file, created))

        for pgn_file, created in sorted(pgn_files, key=lambda x: x[1]):
            game = Game.from_pgn(pgn_file)
            target_path = self.get_organized_path(game, created)
            # print(f"{pgn_file} -> {target_path}")
            target_path.parent.mkdir(parents=True, exist_ok=True)
            pgn_file.rename(target_path)

        return len(pgn_files)

    def get_organized_path(self, game: Game, created: datetime) -> Path:
        game_timestamp = game.get_datetime()
        target_path = self.target_folder

        if game.is_xot():
            target_path /= "xot"
        else:
            target_path /= "normal"

        if game_timestamp:
            return target_path / game_timestamp.strftime("%Y/%m/%d/%H_%M_%S.pgn")

        game_date = game.get_date()

        if created.date() == game_date:
            return target_path / created.strftime("%Y/%m/%d/%H_%M_%S.pgn")

        target_path /= game_date.strftime("%Y/%m/%d")

        for i in count(1):
            potential_path = target_path / f"{i:04}.pgn"

            if not potential_path.exists():
                return potential_path

        raise NotImplementedError  # Unreachable

    def download_from_playok(self) -> int:
        raw_pgns: list[str] = []
        raw_pgn_lines: list[str] = []

        for username in self.playok_usernames:
            response = requests.get(f"https://www.playok.com/p/?uid={username}&g=rv")
            response.raise_for_status()

            lines = response.text.split("\n")

            for line_offset, line in enumerate(lines):
                if line_offset == 0:
                    prev_line_empty = False
                else:
                    prev_line_empty = lines[line_offset - 1].strip() == ""

                # We detect a new PGN by an empty line followed by metadata
                if prev_line_empty and line.startswith("["):
                    raw_pgn = "\n".join(raw_pgn_lines)
                    raw_pgns.append(raw_pgn)
                    raw_pgn_lines = []

                raw_pgn_lines.append(line.strip())

        # Add last PGN of file
        raw_pgn = "\n".join(raw_pgn_lines)
        raw_pgns.append(raw_pgn)

        new_files = 0

        # PGN's come in chronolgical order.
        # Using a reversed loop, we can break out of loop early if we find an existing file,
        # which would be the latest file we got in an earlier run.
        for raw_pgn in reversed(raw_pgns):
            game = Game.from_string(raw_pgn)
            file = self.get_organized_path(game, datetime.now())
            if file.exists():
                break
            new_files += 1
            file.parent.mkdir(exist_ok=True, parents=True)
            file.write_text(raw_pgn)

        return new_files


app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def pgn_organizer() -> None:
    PgnOrganizer()()


if __name__ == "__main__":
    app()
