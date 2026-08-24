-- Deletes boards rows outside the savable disc-count range (internal/db MinSavableDiscs..
-- MaxSavableDiscs, 12..30). Below 12 every position is minimax-derived from the 12-disc rows
-- (internal/book), above 30 nothing ever searches the row -- either way the row is dead weight,
-- and a below-12 one is worse than that: it makes the frontend show a stored evaluation where the
-- minimax cache should be the only answer.
--
-- One-off cleanup for a book written before Repository.AddPositionsInserted enforced the range.
-- Run it against the live book:
--
--   psql "$FLIPPY_POSTGRES_URL" -f scripts/delete_out_of_range_boards.sql
--
-- Reports what it deleted, per disc count. Check the same shape before and after with:
--
--   SELECT disc_count, count(*) FROM boards WHERE disc_count < 12 GROUP BY 1;

BEGIN;

CREATE TEMP TABLE deleted_boards ON COMMIT DROP AS
DELETE FROM boards
WHERE disc_count < 12 OR disc_count > 30
RETURNING disc_count, level;

DO $$
DECLARE
    row_count bigint;
    per_disc text;
BEGIN
    SELECT count(*) INTO row_count FROM deleted_boards;

    IF row_count = 0 THEN
        RAISE NOTICE 'no out-of-range rows found';
        RETURN;
    END IF;

    SELECT string_agg(format('%s discs: %s (%s learned)', disc_count, total, learned), ', '
                      ORDER BY disc_count)
    INTO per_disc
    FROM (
        SELECT disc_count, count(*) AS total, count(*) FILTER (WHERE level > 0) AS learned
        FROM deleted_boards
        GROUP BY disc_count
    ) counts;

    RAISE NOTICE 'deleted % out-of-range rows -- %', row_count, per_disc;
END $$;

COMMIT;
