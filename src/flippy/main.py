import os
import typer

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

from flippy.window import Window  # noqa:E402


def _main(training_filters: list[str] = typer.Option([], "-f")) -> None:
    Window(training_filters).run()


def main() -> None:
    typer.run(_main)
