import datetime
import typer
from math import ceil
from typing import Annotated, Optional

from flippy.config import config
from flippy.db import DB, MIN_LEARN_LEVEL, is_savable_position
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxEvaluations, EdaxRequest
from flippy.othello.game import Game
from flippy.othello.position import Position

app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def info() -> None:
    DB().print_stats()


def get_normalized_pgn_positions() -> set[Position]:
    positions: set[Position] = set()
    pgn_files = list((config.pgn_target_folder() / "normal").rglob("*.pgn"))
    for offset, file in enumerate(pgn_files):
        percentage = 100.0 * offset / len(pgn_files)
        print(
            f"Loading PGN files: {offset+1}/{len(pgn_files)} ({percentage:6.2f}%)\r",
            end="",
        )

        game = Game.from_pgn(file)
        for board in game.boards:
            position = board.position
            positions.add(position.normalized())

    print("Loading PGN files: done." + " " * 20)
    return positions


def learn_boards(
    tuples: list[tuple[Position, int]], learn_level: int
) -> EdaxEvaluations:
    if learn_level % 2 == 1:
        learn_level += 1

    boards = [item[0] for item in tuples]

    request = EdaxRequest(boards, learn_level, source=None)

    before = datetime.datetime.now()
    evaluations = start_evaluation_sync(request)
    after = datetime.datetime.now()

    level_before = set(item[1] for item in tuples)
    seconds_per_board = (after - before).total_seconds() / len(tuples)

    print(
        ",".join(str(level) for level in sorted(level_before))
        + f" -> {learn_level} | {seconds_per_board:7.3f} sec / board"
    )

    return evaluations


def learn_pgn_boards(db: DB, chunk_size: int) -> None:
    print("Loading PGN positions...")
    pgn_positions = get_normalized_pgn_positions()

    # Remove positions that we won't save in DB.
    pgn_positions = {
        position for position in pgn_positions if is_savable_position(position)
    }

    print("Looking up positions in DB")
    evaluations = db.lookup_positions(pgn_positions)

    found_pgn_positions = set(evaluations.values.keys())

    learn_positions = list(pgn_positions - found_pgn_positions)

    for chunk_id in range(ceil(len(learn_positions) / chunk_size)):
        chunk_start = chunk_size * chunk_id
        chunk_end = chunk_size * (chunk_id + 1)
        chunk = learn_positions[chunk_start:chunk_end]
        request = EdaxRequest(chunk, MIN_LEARN_LEVEL, source=None)

        before = datetime.datetime.now()
        learned_evaluations = start_evaluation_sync(request)
        after = datetime.datetime.now()

        seconds_per_board = (after - before).total_seconds() / len(chunk)

        db.update(learned_evaluations)

        print(f"0 -> {MIN_LEARN_LEVEL} | {seconds_per_board:7.3f} sec / board")


@app.command()
def learn(
    chunk_size: Annotated[int, typer.Option("-c", "--chunk-size")] = 100,
    min_learn_level: Annotated[
        Optional[int],
        typer.Option("-l", "--min-learn-level", min=MIN_LEARN_LEVEL, max=32),
    ] = None,
) -> None:
    db = DB()

    learn_pgn_boards(db, chunk_size)

    if min_learn_level is not None:
        while True:
            tuples = db.get_learning_boards_below_level(chunk_size, min_learn_level)

            if not tuples:
                break

            evaluations = learn_boards(tuples, min_learn_level)
            db.update(evaluations)

    while True:
        tuples = db.get_learning_boards(chunk_size)

        if not tuples:
            break

        min_level = min([item[1] for item in tuples])
        tuples = [item for item in tuples if item[1] == min_level]

        evaluations = learn_boards(tuples, min_level + 2)
        db.update(evaluations)


if __name__ == "__main__":
    app()
