import typer
from pathlib import Path
from typing import Annotated

from flippy.othello.game import Game
from flippy.training.file import DEFAULT_TRAINING_FILE_PATH, TrainingFile

app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def add_game(game_file: Annotated[Path, typer.Argument()]) -> None:
    training_file = TrainingFile(DEFAULT_TRAINING_FILE_PATH)
    game = Game.from_pgn(game_file)
    training_file.add_game(game)
    training_file.save()


@app.command()
def show() -> None:
    training_file = TrainingFile(DEFAULT_TRAINING_FILE_PATH)
    training_file.print_ascii_art()


if __name__ == "__main__":
    app()
