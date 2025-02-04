import asyncpg  # type:ignore[import-untyped]
from itertools import count

from flippy.config import get_db_dsn
from flippy.edax.types import EdaxEvaluation
from flippy.othello.position import NormalizedPosition


async def validate_db() -> None:
    counts = await validate_edax_table()
    await validate_edax_stats_table(counts)


async def validate_edax_table() -> dict[tuple[int, int], int]:
    conn = await asyncpg.connect(get_db_dsn())

    limit = 50_000

    counts: dict[tuple[int, int], int] = {}
    checked_rows = 0

    for offset in count(0, limit):
        rows = await conn.fetch(
            """
            SELECT position, level, depth, confidence, score, best_moves
            FROM edax
            ORDER BY position
            LIMIT $1 OFFSET $2
            """,
            limit,
            offset,
        )

        if not rows:
            break

        for row in rows:
            try:
                normalized = NormalizedPosition.from_bytes(row["position"])

                EdaxEvaluation(
                    position=normalized.to_position(),
                    level=row["level"],
                    depth=row["depth"],
                    confidence=row["confidence"],
                    score=row["score"],
                    best_moves=row["best_moves"],
                )
            except Exception as e:
                print(f"Error at position {row['position']}: {e}")

            level = row["level"]
            depth = row["depth"]

            if (level, depth) not in counts:
                counts[(level, depth)] = 0

            counts[(level, depth)] += 1

        checked_rows += len(rows)
        print(f"Checked {checked_rows} rows")

    return counts


async def validate_edax_stats_table(counts: dict[tuple[int, int], int]) -> None:
    # TODO implement this once the edax stats table is implemented
    _ = counts
