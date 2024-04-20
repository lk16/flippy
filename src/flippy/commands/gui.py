import os
import typer
from pathlib import Path
from typing import Annotated, Optional

from flippy.arguments import (
    Arguments,
    PGNArguments,
    PositionFrequencyArguments,
    TrainingArguments,
)

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

from flippy.window import Window  # noqa:E402

app = typer.Typer()


@app.command()
def main(
    filters: Annotated[list[str], typer.Option("-f")] = [],
    top: Annotated[Optional[int], typer.Option("-t")] = None,
    lost_only: Annotated[bool, typer.Option("-l")] = False,
    most_recent: Annotated[Optional[int], typer.Option("-r")] = None,
    pgn_file: Annotated[Optional[Path], typer.Option("-p")] = None,
) -> None:
    loader_args = TrainingArguments(filters, top)
    freq_args = PositionFrequencyArguments(lost_only, most_recent)
    pgn_args = PGNArguments(pgn_file)
    args = Arguments(loader_args, freq_args, pgn_args)

    Window(args).run()


if __name__ == "__main__":
    app()
