import typer
import uvicorn
from pathlib import Path

from flippy.book.client import BookLearningClient
from flippy.book.load_pgn import load_pgn as load_pgn_
from flippy.book.load_wthor import load_wthor as load_wthor_
from flippy.db import DB

app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def info() -> None:
    DB().print_edax_stats()


@app.command()
def server() -> None:
    uvicorn.run("flippy.book.server:app", host="0.0.0.0", port=7777)


@app.command()
def client() -> None:
    BookLearningClient().run()


@app.command()
def load_pgn() -> None:
    load_pgn_()


@app.command()
def load_wthor(filenames: list[Path]) -> None:
    load_wthor_(filenames)


if __name__ == "__main__":
    app()
