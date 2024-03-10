import os
from typing import Optional
import typer
from flippy.mode.training.loader import ExerciseLoaderArgs

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

from flippy.window import Window  # noqa:E402


def _main(
    filters: list[str] = typer.Option([], "-f"),
    top: Optional[int] = typer.Option(None, "-t"),
) -> None:
    loader_args = ExerciseLoaderArgs(filters, top)
    Window(loader_args).run()


def main() -> None:
    typer.run(_main)
