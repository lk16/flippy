from piccolo.apps.migrations.auto.migration_manager import MigrationManager
from piccolo.table import Table

ID = "2024-04-26T21:12:35:409564"
VERSION = "1.5.0"
DESCRIPTION = ""


async def forwards() -> MigrationManager:
    manager = MigrationManager(
        migration_id=ID, app_name="piccolo_app", description=DESCRIPTION
    )

    async def raw_func() -> None:
        queries = [
            """
            CREATE TABLE openings (
                me bytea,
                opp bytea,
                disc_count INTEGER,
                level INTEGER,
                depth INTEGER,
                confidence INTEGER,
                score INTEGER,
                learn_priority INTEGER,
                best_moves INTEGER[]
            );
            """,
            "CREATE UNIQUE INDEX idx_openings_me_opp ON openings (me, opp);",
            "CREATE INDEX idx_openings_disc_count ON openings (disc_count);",
            "CREATE INDEX idx_openings_level ON openings (level);",
            "CREATE INDEX idx_openings_learn_priority ON openings (learn_priority);",
            "CREATE INDEX idx_openings_learn_depth ON openings (depth);",
            "CREATE INDEX idx_openings_disc_count_level ON openings (disc_count, level);",
        ]

        for query in queries:
            await Table.raw(query)

    async def raw_backwards_func() -> None:
        await Table.raw("DROP TABLE openings;")  # This also drops all indexes

    manager.add_raw(raw_func)
    manager.add_raw_backwards(raw_backwards_func)
    return manager
