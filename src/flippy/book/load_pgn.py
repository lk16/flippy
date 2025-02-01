import json
import requests
from datetime import datetime, timedelta
from math import ceil

from flippy import PROJECT_ROOT
from flippy.book import MIN_LEARN_LEVEL, is_savable_position
from flippy.book.models import SerializedEvaluation
from flippy.config import PgnConfig, get_book_server_token, get_book_server_url
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxRequest
from flippy.othello.game import Game
from flippy.othello.position import Position

LEARN_CHUNK_SIZE = 100

PGN_JSON_PATH = PROJECT_ROOT / ".flippy/pgn.json"


def load_pgn() -> None:
    pgn_config = PgnConfig()

    positions: set[Position] = set()

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
            f"Loading PGN files: {offset+1}/{len(pgn_files)} ({percentage:6.2f}%)\r",
            end="",
        )

    print()

    learn_new_positions(positions)

    data = {"last_read_pgn": str(pgn_files[-1].relative_to(prefix))}
    PGN_JSON_PATH.parent.mkdir(parents=True, exist_ok=True)
    json.dump(data, PGN_JSON_PATH.open("w"))


def learn_new_positions(positions: set[Position]) -> None:
    # Remove positions that we won't save in DB.
    pgn_positions = {
        position for position in positions if is_savable_position(position)
    }

    print(f"Looking up {len(pgn_positions)} positions in DB")

    # Convert positions to list for chunking
    position_list = list(pgn_positions)
    found_pgn_positions: set[Position] = set()

    server_url = get_book_server_url()
    token = get_book_server_token()

    # Fetch evaluations in chunks of 100
    for i in range(0, len(position_list), 100):
        chunk = position_list[i : i + 100]
        response = requests.get(
            f"{server_url}/api/positions",
            json=[pos.to_api() for pos in chunk],
            headers={"x-token": token},
        )

        response.raise_for_status()
        parsed = [SerializedEvaluation.model_validate(item) for item in response.json()]

        found_pgn_positions.update([Position.from_api(pos.position) for pos in parsed])

        # Print progress
        print(f"Fetched positions {i + len(chunk)}/{len(position_list)}\r", end="")

    print()

    learn_positions = list(pgn_positions - found_pgn_positions)

    total_seconds = 0.0

    for chunk_id in range(ceil(len(learn_positions) / LEARN_CHUNK_SIZE)):
        chunk_start = LEARN_CHUNK_SIZE * chunk_id
        chunk_end = LEARN_CHUNK_SIZE * (chunk_id + 1)
        chunk = learn_positions[chunk_start:chunk_end]
        request = EdaxRequest(chunk, MIN_LEARN_LEVEL, source=None)

        before = datetime.now()
        learned_evaluations = start_evaluation_sync(request)
        after = datetime.now()

        seconds = (after - before).total_seconds()
        total_seconds += seconds

        computed_positions = min(chunk_end, len(learn_positions))
        average = total_seconds / computed_positions

        eta = datetime.now() + timedelta(
            seconds=average * (len(learn_positions) - computed_positions)
        )

        payload = [
            SerializedEvaluation.from_evaluation(eval).model_dump()
            for eval in learned_evaluations.values.values()
        ]

        response = requests.post(
            f"{server_url}/api/evaluations", json=payload, headers={"x-token": token}
        )
        response.raise_for_status()

        print(
            f"new positions @ lvl {MIN_LEARN_LEVEL} | {min(chunk_end, len(learn_positions))}/{len(learn_positions)} "
            + f"| {seconds:7.3f} sec "
            + f"| ETA {eta.strftime('%Y-%m-%d %H:%M:%S')}"
        )
