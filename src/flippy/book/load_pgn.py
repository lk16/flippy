import json
from datetime import datetime, timedelta
from math import ceil

from flippy import PROJECT_ROOT
from flippy.book import MIN_LEARN_LEVEL
from flippy.book.api_client import APIClient
from flippy.config import PgnConfig
from flippy.edax.process import evaluate_blocking
from flippy.edax.types import EdaxRequest
from flippy.othello.game import Game
from flippy.othello.position import NormalizedPosition

LEARN_CHUNK_SIZE = 100

PGN_JSON_PATH = PROJECT_ROOT / ".flippy/pgn.json"


def load_pgn() -> None:
    pgn_config = PgnConfig()

    positions: set[NormalizedPosition] = set()

    prefix = pgn_config.target_folder / "normal"
    pgn_files = sorted(prefix.rglob("*.pgn"))

    last_read_pgn: str | None = None
    if PGN_JSON_PATH.exists():
        data = json.load(PGN_JSON_PATH.open())
        last_read_pgn = data["last_read_pgn"]

    if last_read_pgn is not None:
        pgn_files = [
            file for file in pgn_files if str(file.relative_to(prefix)) > last_read_pgn
        ]

    if not pgn_files:
        print("No new PGN files to load.")
        return

    for offset, file in enumerate(pgn_files):
        game = Game.from_pgn(file)
        positions.update(game.get_normalized_positions(add_children=True))

        percentage = 100.0 * (offset + 1) / len(pgn_files)
        print(
            f"Loading PGN files: {offset + 1}/{len(pgn_files)} ({percentage:6.2f}%)\r",
            end="",
        )

    print()

    learn_new_positions(positions)

    data = {"last_read_pgn": str(pgn_files[-1].relative_to(prefix))}
    PGN_JSON_PATH.parent.mkdir(parents=True, exist_ok=True)
    json.dump(data, PGN_JSON_PATH.open("w"))


def learn_new_positions(positions: set[NormalizedPosition]) -> None:
    # Remove positions that we won't save in DB.
    pgn_positions = {position for position in positions if position.is_db_savable()}

    print(f"Looking up {len(pgn_positions)} positions in DB")

    api_client = APIClient()

    found_evaluations = api_client.lookup_positions(pgn_positions)
    found_positions = found_evaluations.keys()

    learn_positions = list(pgn_positions - set(found_positions))

    total_seconds = 0.0

    for chunk_id in range(ceil(len(learn_positions) / LEARN_CHUNK_SIZE)):
        chunk_start = LEARN_CHUNK_SIZE * chunk_id
        chunk_end = LEARN_CHUNK_SIZE * (chunk_id + 1)
        chunk = learn_positions[chunk_start:chunk_end]
        request = EdaxRequest(set(chunk), MIN_LEARN_LEVEL, source=None)

        before = datetime.now()
        edax_evaluations = evaluate_blocking(request)
        after = datetime.now()

        seconds = (after - before).total_seconds()
        total_seconds += seconds

        computed_positions = min(chunk_end, len(learn_positions))
        average = total_seconds / computed_positions

        eta = datetime.now() + timedelta(
            seconds=average * (len(learn_positions) - computed_positions)
        )

        api_client.save_learned_evaluations(edax_evaluations.values())

        print(
            f"new positions @ lvl {MIN_LEARN_LEVEL} | {min(chunk_end, len(learn_positions))}/{len(learn_positions)} "
            + f"| {seconds:7.3f} sec "
            + f"| ETA {eta.strftime('%Y-%m-%d %H:%M:%S')}"
        )
