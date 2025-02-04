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

            level = row["level"]

            if (disc_count, level) not in counts:
                counts[(disc_count, level)] = 0

            counts[(disc_count, level)] += 1

        checked_rows += len(rows)
        print(f"Checked {checked_rows} rows")

    return counts


async def validate_edax_stats_table(expected_stats: dict[tuple[int, int], int]) -> None:
    conn = await asyncpg.connect(get_db_dsn())

    # Fetch all entries from edax_stats
    rows = await conn.fetch("SELECT disc_count, level, count FROM edax_stats")

    # Convert DB stats into comparable format
    stats: dict[tuple[int, int], int] = {}
    for row in rows:
        stats[(row["disc_count"], row["level"])] = row["count"]

    # Compare stats with expected stats
    for key in set(stats) | set(expected_stats):
        actual = stats.get(key, 0)
        expected = expected_stats.get(key, 0)
        if actual != expected:
            disc_count, level = key
            print(f"Mismatch for disc_count={disc_count}, level={level}:")
            print(f"  Expected: {expected}")
            print(f"  Actual: {actual}")

    await conn.close()


async def recalculate_edax_stats_table() -> None:
    conn = await asyncpg.connect(get_db_dsn())
    await conn.execute("DELETE FROM edax_stats")

    rows = await conn.fetch(
        "SELECT disc_count, level, COUNT(*) FROM edax GROUP BY disc_count, level"
    )
    for row in rows:
        await conn.execute(
            "INSERT INTO edax_stats (disc_count, level, count) VALUES ($1, $2, $3)",
            row["disc_count"],
            row["level"],
            row["count"],
        )
