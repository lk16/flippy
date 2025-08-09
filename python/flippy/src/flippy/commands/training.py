import typer
from pathlib import Path
from typing import Annotated

from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.game import Game
from flippy.othello.position import Position
from flippy.training.file import DEFAULT_TRAINING_FILE_PATH, TrainingFile

app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def add_game(
    game_files: Annotated[list[Path], typer.Argument(help="One or more PGN files")],
) -> None:
    training_file = TrainingFile(DEFAULT_TRAINING_FILE_PATH)

    for game_file in game_files:
        print(f"Adding game from {game_file}")
        game = Game.from_pgn(game_file)
        training_file.add_game(game)

    training_file.save()


@app.command()
def show() -> None:
    training_file = TrainingFile(DEFAULT_TRAINING_FILE_PATH)
    training_file.print_ascii_art()


@app.command()
def list_exercises() -> None:
    training_file = TrainingFile(DEFAULT_TRAINING_FILE_PATH)
    training_file.list_exercises()


@app.command()
def show_board(
    board_string: Annotated[str, typer.Argument()],
    white_turn: Annotated[bool, typer.Option("-w")] = False,
) -> None:
    if len(board_string) != 32:
        raise typer.BadParameter("Board string must be 32 characters long")

    if white_turn:
        turn = WHITE
    else:
        turn = BLACK

    me = int(board_string[:16], 16)
    opp = int(board_string[16:], 16)

    Board(Position(me, opp), turn).show()


if __name__ == "__main__":
    app()
