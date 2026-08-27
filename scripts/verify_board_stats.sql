-- Checks that board_stats agrees with the boards table it counts. The triggers migration 000007
-- installs keep the two in step transactionally, so a mismatch means a write got past them -- a
-- trigger dropped by hand, or rows loaded with session_replication_role = replica.
--
--   psql "$FLIPPY_POSTGRES_URL" -f scripts/verify_board_stats.sql
--
-- Reads the whole book, so it takes about as long as the GROUP BY the counts exist to avoid: run it
-- once after migrating, not on a schedule. Raises an exception listing the disagreeing cells;
-- a clean run prints how many it checked.

DO $$
DECLARE
    cells bigint;
    offenders text;
BEGIN
    WITH from_boards AS (
        SELECT disc_count, level, count(*) AS n FROM boards GROUP BY disc_count, level
    ), compared AS (
        -- A cell emptied again keeps a zero row rather than disappearing, so zero and missing agree.
        SELECT disc_count, level, coalesce(from_boards.n, 0) AS actual, coalesce(stats.count, 0) AS stored
        FROM from_boards FULL JOIN board_stats stats USING (disc_count, level)
    )
    SELECT count(*) FILTER (WHERE actual > 0),
           string_agg(format('%s discs level %s: board_stats says %s, boards has %s',
                             disc_count, level, stored, actual), ', '
                      ORDER BY disc_count, level) FILTER (WHERE actual <> stored)
    INTO cells, offenders
    FROM compared;

    IF offenders IS NOT NULL THEN
        RAISE EXCEPTION 'board_stats disagrees with boards -- %', offenders;
    END IF;

    RAISE NOTICE 'board_stats matches boards across % cells', cells;
END $$;
