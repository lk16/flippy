import asyncio
import typer
import uvicorn
from pathlib import Path

from flippy.book.learn_client import BookLearningClient
from flippy.book.load_pgn import load_pgn as load_pgn_
from flippy.book.load_wthor import load_wthor as load_wthor_
from flippy.book.validate_db import validate_db
from flippy.config import BookServerConfig

app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def server(
    reload: bool = typer.Option(False),
    workers: int = typer.Option(1),
) -> None:
    config = BookServerConfig()
    uvicorn.run(
        "flippy.book.server:app",
        host=config.host,
        port=config.port,
        reload=reload,
        workers=workers,
    )


@app.command()
def client() -> None:
    BookLearningClient().run()


@app.command()
def load_pgn() -> None:
    load_pgn_()


@app.command()
def load_wthor(filenames: list[Path]) -> None:
    load_wthor_(filenames)


@app.command()
def validate() -> None:
    asyncio.run(validate_db())


if __name__ == "__main__":
    app()
