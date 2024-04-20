import os
import typer
from pathlib import Path
from typing import Optional

from flippy.arguments import (
    Arguments,
    PGNArguments,
    PositionFrequencyArguments,
    TrainingArguments,
)
from flippy.commands.pgn_analyzer import PgnAnanlyzer
from flippy.commands.pgn_organizer import PgnOrganizer
from flippy.commands.recent_games import RecentGames

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

from flippy.window import Window  # noqa:E402

# TODO make Typer commands better so --help text works


def gui() -> None:
    def command(
        filters: list[str] = typer.Option([], "-f"),
        top: Optional[int] = typer.Option(None, "-t"),
        lost_only: bool = typer.Option(False, "-l"),
        most_recent: Optional[int] = typer.Option(None, "-r"),
        pgn_file: Optional[Path] = typer.Option(None, "-p"),
    ) -> None:
        loader_args = TrainingArguments(filters, top)
        freq_args = PositionFrequencyArguments(lost_only, most_recent)
        pgn_args = PGNArguments(pgn_file)
        args = Arguments(loader_args, freq_args, pgn_args)

        Window(args).run()

    typer.run(command)


def organize_pgn() -> None:
    def command() -> None:
        PgnOrganizer()()

    typer.run(command)


def recent_games() -> None:
    def command(count: int = typer.Option(20, "-n")) -> None:
        RecentGames(count)()

    typer.run(command)


def analyze_pgn() -> None:
    def command(file: Path, level: int = typer.Option(18, "-l")) -> None:
        PgnAnanlyzer(file, level)()

    typer.run(command)
