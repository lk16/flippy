import os
from typing import Optional
import typer
from flippy.arguments import (
    Arguments,
    TrainingArguments,
    PositionFrequencyArguments,
)
from flippy.pgn_organizer import PgnOrganizer

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

from flippy.window import Window  # noqa:E402


def gui() -> None:
    def command(
        filters: list[str] = typer.Option([], "-f"),
        top: Optional[int] = typer.Option(None, "-t"),
        lost_only: bool = typer.Option(False, "-l"),
        most_recent: Optional[int] = typer.Option(None, "-r"),
    ) -> None:
        loader_args = TrainingArguments(filters, top)
        freq_args = PositionFrequencyArguments(lost_only, most_recent)
        args = Arguments(loader_args, freq_args)

        Window(args).run()

    typer.run(command)


def organize_pgn() -> None:
    def command() -> None:
        PgnOrganizer()()

    typer.run(command)
