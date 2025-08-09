# Don't complain about setting env var in the middle of imports.
# ruff: noqa: E402

import os
import typer
from pathlib import Path
from typing import Annotated, Optional, Type

# Disable pygame start-up text.
# This needs to be before first pygame import.
os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

from flippy.arguments import (
    Arguments,
    PGNArguments,
    PositionFrequencyArguments,
)
from flippy.mode.base import BaseMode
from flippy.mode.evaluate import EvaluateMode
from flippy.mode.frequency import FrequencyMode
from flippy.mode.game import GameMode
from flippy.mode.pgn import PGNMode
from flippy.mode.training import TrainingMode
from flippy.mode.watch import WatchMode
from flippy.window import Window

app = typer.Typer(pretty_exceptions_enable=False)

MODES = {
    "evaluate": EvaluateMode,
    "frequency": FrequencyMode,
    "game": GameMode,
    "pgn": PGNMode,
    "training": TrainingMode,
    "watch": WatchMode,
}


@app.command()
def main(
    filters: Annotated[list[str], typer.Option("-f")] = [],
    top: Annotated[Optional[int], typer.Option("-t")] = None,
    lost_only: Annotated[bool, typer.Option("-l")] = False,
    most_recent: Annotated[Optional[int], typer.Option("-r")] = None,
    pgn_file: Annotated[Optional[Path], typer.Option("-p")] = None,
    oq_string: Annotated[Optional[str], typer.Option("-o")] = None,
    mode_name: Annotated[str, typer.Option("-m")] = "game",
) -> None:
    freq_args = PositionFrequencyArguments(lost_only, most_recent)
    pgn_args = PGNArguments(pgn_file, oq_string)
    args = Arguments(freq_args, pgn_args)

    try:
        mode_type: Type[BaseMode] = MODES[mode_name]
    except KeyError:
        print(f"Mode not found: {mode_name}")
        print("Available modes: ")
        for name in sorted(MODES.keys()):
            print(f"- {name}")
        exit(1)

    Window(mode_type, args).run()


if __name__ == "__main__":
    app()
