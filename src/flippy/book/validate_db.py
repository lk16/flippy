import asyncpg  # type:ignore[import-untyped]
from itertools import count

from flippy.config import get_db_dsn
from flippy.edax.types import EdaxEvaluation
from flippy.othello.position import NormalizedPosition


async def validate_db() -> None:
    await validate_edax_table()


async def validate_edax_table() -> None:
    conn = await asyncpg.connect(get_db_dsn())

    limit = 50_000

    checked_rows = 0

    for offset in count(0, limit):
        rows = await conn.fetch(
            """
            SELECT position, disc_count, level, depth, confidence, score, best_moves
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

                position = normalized.to_position()

                EdaxEvaluation(
                    position=position,
                    level=row["level"],
                    depth=row["depth"],
                    confidence=row["confidence"],
                    score=row["score"],
                    best_moves=row["best_moves"],
                )
                disc_count = row["disc_count"]

                if disc_count != position.count_discs():
                    raise ValueError(
                        f"Disc count mismatch for position {row['position']}: "
                        f"{disc_count} != {position.count_discs()}"
                    )

            except Exception as e:
                print(f"Error at position {row['position']}: {e}")
                continue

        checked_rows += len(rows)
        print(f"Checked {checked_rows} rows")
