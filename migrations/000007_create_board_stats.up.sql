-- Position counts per (disc_count, level), kept by triggers on boards. The counts used to live in
-- a Redis hash that the server incremented on its own writes and fully resynced every 15 minutes;
-- anything that wrote rows without going through the server (loader, operator SQL) drifted until
-- that resync, and increments racing the resync's swap were dropped. Counting in the same
-- transaction as the row change makes the numbers exact for every writer instead.
--
-- A GROUP BY over the boards table is not an alternative: Postgres has no loose index scan, so
-- counting 14M rows into ~200 cells reads every index entry, and the book only grows.

CREATE TABLE board_stats (
    disc_count smallint NOT NULL,
    level smallint NOT NULL,
    count bigint NOT NULL,
    PRIMARY KEY (disc_count, level)
);

-- Statement-level with transition tables: a loader run inserting millions of rows costs one
-- aggregate over the inserted rows and one upsert per (disc_count, level) cell, not one per row.
-- ORDER BY makes concurrent statements lock the cells they touch in the same order, so two
-- transactions spanning several cells can't deadlock against each other.
CREATE FUNCTION board_stats_apply() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO board_stats (disc_count, level, count)
        SELECT disc_count, level, count(*)
        FROM added
        GROUP BY disc_count, level
        ORDER BY disc_count, level
        ON CONFLICT (disc_count, level) DO UPDATE SET count = board_stats.count + EXCLUDED.count;
    ELSIF TG_OP = 'DELETE' THEN
        INSERT INTO board_stats (disc_count, level, count)
        SELECT disc_count, level, -count(*)
        FROM removed
        GROUP BY disc_count, level
        ORDER BY disc_count, level
        ON CONFLICT (disc_count, level) DO UPDATE SET count = board_stats.count + EXCLUDED.count;
    ELSE
        -- Only rows that moved between cells matter: a score-only save nets zero and writes nothing.
        INSERT INTO board_stats (disc_count, level, count)
        SELECT disc_count, level, sum(delta)
        FROM (
            SELECT disc_count, level, -1::bigint AS delta FROM removed
            UNION ALL
            SELECT disc_count, level, 1::bigint FROM added
        ) moved
        GROUP BY disc_count, level
        HAVING sum(delta) <> 0
        ORDER BY disc_count, level
        ON CONFLICT (disc_count, level) DO UPDATE SET count = board_stats.count + EXCLUDED.count;
    END IF;

    RETURN NULL;
END $$;

CREATE TRIGGER board_stats_insert AFTER INSERT ON boards
    REFERENCING NEW TABLE AS added
    FOR EACH STATEMENT EXECUTE FUNCTION board_stats_apply();

CREATE TRIGGER board_stats_update AFTER UPDATE ON boards
    REFERENCING OLD TABLE AS removed NEW TABLE AS added
    FOR EACH STATEMENT EXECUTE FUNCTION board_stats_apply();

CREATE TRIGGER board_stats_delete AFTER DELETE ON boards
    REFERENCING OLD TABLE AS removed
    FOR EACH STATEMENT EXECUTE FUNCTION board_stats_apply();

-- TRUNCATE bypasses the row triggers entirely, so it gets its own.
CREATE FUNCTION board_stats_truncate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM board_stats;
    RETURN NULL;
END $$;

CREATE TRIGGER board_stats_truncate AFTER TRUNCATE ON boards
    FOR EACH STATEMENT EXECUTE FUNCTION board_stats_truncate();

-- The backfill runs last, and inside the same transaction as the CREATE TRIGGERs above: those hold
-- an ACCESS EXCLUSIVE lock on boards until this migration commits, so no write can slip between
-- what the count below sees and the first write a trigger catches. Backfilling first would miss a
-- row inserted in that gap. The price is that writes to boards block for the length of the count.
INSERT INTO board_stats (disc_count, level, count)
SELECT disc_count, level, count(*)
FROM boards
GROUP BY disc_count, level;
