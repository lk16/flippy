import datetime
import typer
from math import ceil

from flippy.config import config
from flippy.db import DB, MAX_SAVABLE_DISCS, MIN_LEARN_LEVEL, is_savable_position
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxEvaluations, EdaxRequest
from flippy.othello.game import Game
from flippy.othello.position import Position

LEARN_CHUNK_SIZE = 20

app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def info() -> None:
    DB().print_edax_stats()


def get_normalized_pgn_positions() -> set[Position]:
    positions: set[Position] = set()
    pgn_files = list((config.pgn_target_folder() / "normal").rglob("*.pgn"))
    for offset, file in enumerate(pgn_files):
        game = Game.from_pgn(file)
        for board in game.boards:
            position = board.position
            positions.add(position.normalized())

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


def learn_pgn_boards(db: DB) -> None:
    pgn_positions = get_normalized_pgn_positions()

    # Remove positions that we won't save in DB.
    pgn_positions = {
        position for position in pgn_positions if is_savable_position(position)
    }

    print("Looking up positions in DB")
    evaluations = db.lookup_edax_positions(pgn_positions)

    found_pgn_positions = set(evaluations.values.keys())

    learn_positions = list(pgn_positions - found_pgn_positions)

    for chunk_id in range(ceil(len(learn_positions) / LEARN_CHUNK_SIZE)):
        chunk_start = LEARN_CHUNK_SIZE * chunk_id
        chunk_end = LEARN_CHUNK_SIZE * (chunk_id + 1)
        chunk = learn_positions[chunk_start:chunk_end]
        request = EdaxRequest(chunk, MIN_LEARN_LEVEL, source=None)

        before = datetime.datetime.now()
        learned_evaluations = start_evaluation_sync(request)
        after = datetime.datetime.now()

        seconds_per_board = (after - before).total_seconds() / len(chunk)

        db.update_edax_evaluations(learned_evaluations)

        print(f"0 -> {MIN_LEARN_LEVEL} | {seconds_per_board:7.3f} sec / board")


def get_learn_level(disc_count: int) -> int:
    if disc_count <= 10:
        return 36

    if disc_count <= 16:
        return 34

    return 32


@app.command()
def learn() -> None:
    db = DB()

    learn_pgn_boards(db)

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


if __name__ == "__main__":
    app()
