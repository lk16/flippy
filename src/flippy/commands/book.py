import datetime
import typer
import uvicorn
from math import ceil
from pathlib import Path

from flippy.book import get_learn_level
from flippy.book.client import BookLearningClient
from flippy.config import PGN_TARGET_FOLDER
from flippy.db import DB, MAX_SAVABLE_DISCS, MIN_LEARN_LEVEL, is_savable_position
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxEvaluations, EdaxRequest
from flippy.othello.game import Game
from flippy.othello.position import Position
from flippy.othello.wthor import Wthor

LEARN_CHUNK_SIZE = 100

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


# TODO add separate command for loading PGN files

# TODO remove most code below


def get_normalized_pgn_positions() -> set[Position]:
    positions: set[Position] = set()
    pgn_files = list((PGN_TARGET_FOLDER / "normal").rglob("*.pgn"))

    for offset, file in enumerate(pgn_files):
        game = Game.from_pgn(file)
        positions.update(game.get_normalized_positions(add_children=True))

        percentage = 100.0 * (offset + 1) / len(pgn_files)
        print(
            f"Loading PGN files: {offset+1}/{len(pgn_files)} ({percentage:6.2f}%)\r",
            end="",
        )

    print()
    return positions


def learn_position(position: Position, level: int) -> tuple[EdaxEvaluations, float]:
    request = EdaxRequest([position], level, source=None)

    before = datetime.datetime.now()
    evaluations = start_evaluation_sync(request)
    after = datetime.datetime.now()

    seconds_per_board = (after - before).total_seconds()

    return evaluations, seconds_per_board


def learn_new_positions(db: DB, positions: set[Position]) -> None:
    # Remove positions that we won't save in DB.
    pgn_positions = {
        position for position in positions if is_savable_position(position)
    }

    print("Looking up positions in DB")
    evaluations = db.lookup_edax_positions(pgn_positions)

    found_pgn_positions = set(evaluations.values.keys())

    learn_positions = list(pgn_positions - found_pgn_positions)

    total_seconds = 0.0

    for chunk_id in range(ceil(len(learn_positions) / LEARN_CHUNK_SIZE)):
        chunk_start = LEARN_CHUNK_SIZE * chunk_id
        chunk_end = LEARN_CHUNK_SIZE * (chunk_id + 1)
        chunk = learn_positions[chunk_start:chunk_end]
        request = EdaxRequest(chunk, MIN_LEARN_LEVEL, source=None)

        before = datetime.datetime.now()
        learned_evaluations = start_evaluation_sync(request)
        after = datetime.datetime.now()

        seconds = (after - before).total_seconds()
        total_seconds += seconds

        computed_positions = min(chunk_end, len(learn_positions))
        average = total_seconds / computed_positions

        eta = datetime.datetime.now() + datetime.timedelta(
            seconds=average * (len(learn_positions) - computed_positions)
        )

        db.update_edax_evaluations(learned_evaluations)

        print(
            f"new positions @ lvl {MIN_LEARN_LEVEL} | {min(chunk_end, len(learn_positions))}/{len(learn_positions)} "
            + f"| {seconds:7.3f} sec "
            + f"| ETA {eta.strftime('%Y-%m-%d %H:%M:%S')}"
        )


@app.command()
def learn() -> None:
    db = DB()

    pgn_positions = get_normalized_pgn_positions()
    learn_new_positions(db, pgn_positions)

    for disc_count in range(4, MAX_SAVABLE_DISCS + 1):
        learn_level = get_learn_level(disc_count)

        positions = db.get_boards_with_disc_count_below_level(disc_count, learn_level)

        total_seconds = 0.0

        for i, position in enumerate(positions):
            evaluations, seconds = learn_position(position, learn_level)
            db.update_edax_evaluations(evaluations)

            total_seconds += seconds
            average = total_seconds / (i + 1)

            eta = datetime.datetime.now() + datetime.timedelta(
                seconds=average * (len(positions) - (i + 1))
            )

            print(
                f"{disc_count} discs @ lvl {learn_level} | {i+1}/{len(positions)} | {seconds:7.3f} sec "
                + f"| ETA {eta.strftime('%Y-%m-%d %H:%M:%S')}"
            )


@app.command()
def import_wthor(filenames: list[Path]) -> None:
    positions: set[Position] = set()
    games: list[Game] = []

    print("Loading files.")

    for i, filename in enumerate(filenames):
        games += Wthor(filename).get_games()
        print(f"Loaded {i+1}/{len(filenames)} files.")

    for i, game in enumerate(games):
        positions.update(game.get_normalized_positions())

        if (i + 1) % 100 == 0 or i == len(games) - 1:
            print(
                f"Loaded game {i+1}/{len(games)} | {len(positions)} unique positions."
            )

    print(f"Found {len(games)} games with {len(positions)} unique positions.")

    db = DB()
    learn_new_positions(db, positions)


if __name__ == "__main__":
    app()
