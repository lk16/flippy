import typer
from typing import Annotated

from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.position import Position
from flippy.training.file import DEFAULT_TRAINING_SCORE_FILE_PATH, Training

app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def show() -> None:
    Training(DEFAULT_TRAINING_SCORE_FILE_PATH).show()


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
