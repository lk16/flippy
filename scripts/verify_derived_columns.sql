-- Verifies that boards.depth and boards.confidence hold exactly what (disc_count, level) implies,
-- i.e. that migration 000002 drops them without losing information. Run it against the live book
-- BEFORE migrating:
--
--   psql "$FLIPPY_POSTGRES_URL" -f scripts/verify_derived_columns.sql
--
-- It defines edax_depth/edax_confidence, then raises an exception listing the offenders if any
-- learned row disagrees. A clean run prints the row count it checked and leaves the two functions
-- behind; drop them once done:
--
--   DROP FUNCTION edax_depth(int, int), edax_confidence(int, int), edax_search_params(int, int);
--
-- The same functions rebuild the columns after a `migrate down` (see the down migration).
--
-- edax_search_params is a SQL transcription of edax's search_global_init (search.c:161-346), the
-- same mapping internal/edax.SearchParams ports to Go; TestVerifyDerivedColumnsScript runs this
-- file and compares both over every (disc_count, level) pair, so the two cannot drift.

CREATE OR REPLACE FUNCTION edax_search_params(disc_count int, level int, OUT depth int, OUT confidence int)
AS $$
DECLARE
    empties int := 64 - disc_count;
    selectivity int := 5;  -- NO_SELECTIVITY: full width
BEGIN
    depth := empties;

    IF level <= 0 THEN
        depth := 0;
    ELSIF level <= 10 THEN
        IF empties > 2 * level THEN depth := level; END IF;
    ELSIF level <= 12 THEN
        IF empties <= 21 THEN NULL;
        ELSIF empties <= 24 THEN selectivity := 3;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level <= 18 THEN
        IF empties <= 21 THEN NULL;
        ELSIF empties <= 24 THEN selectivity := 3;
        ELSIF empties <= 27 THEN selectivity := 1;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level <= 21 THEN
        IF empties <= 24 THEN NULL;
        ELSIF empties <= 27 THEN selectivity := 3;
        ELSIF empties <= 30 THEN selectivity := 1;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level <= 24 THEN
        IF empties <= 24 THEN NULL;
        ELSIF empties <= 27 THEN selectivity := 4;
        ELSIF empties <= 30 THEN selectivity := 2;
        ELSIF empties <= 33 THEN selectivity := 0;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level <= 27 THEN
        IF empties <= 27 THEN NULL;
        ELSIF empties <= 30 THEN selectivity := 3;
        ELSIF empties <= 33 THEN selectivity := 1;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level < 30 THEN
        IF empties <= 27 THEN NULL;
        ELSIF empties <= 30 THEN selectivity := 4;
        ELSIF empties <= 33 THEN selectivity := 2;
        ELSIF empties <= 36 THEN selectivity := 0;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level <= 31 THEN
        IF empties <= 30 THEN NULL;
        ELSIF empties <= 33 THEN selectivity := 3;
        ELSIF empties <= 36 THEN selectivity := 1;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level <= 33 THEN
        IF empties <= 30 THEN NULL;
        ELSIF empties <= 33 THEN selectivity := 4;
        ELSIF empties <= 36 THEN selectivity := 2;
        ELSIF empties <= 39 THEN selectivity := 0;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level <= 35 THEN
        IF empties <= 30 THEN NULL;
        ELSIF empties <= 33 THEN selectivity := 4;
        ELSIF empties <= 36 THEN selectivity := 3;
        ELSIF empties <= 39 THEN selectivity := 1;
        ELSE depth := level; selectivity := 0;
        END IF;
    ELSIF level < 60 THEN
        IF empties <= level - 6 THEN NULL;
        ELSIF empties <= level - 3 THEN selectivity := 4;
        ELSIF empties <= level THEN selectivity := 3;
        ELSIF empties <= level + 3 THEN selectivity := 2;
        ELSIF empties <= level + 6 THEN selectivity := 1;
        ELSIF empties <= level + 9 THEN selectivity := 0;
        ELSE depth := level; selectivity := 0;
        END IF;
    END IF;
    -- level >= 60: exact solve at full width, the initial values.

    confidence := (ARRAY[73, 87, 95, 98, 99, 100])[selectivity + 1];
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE OR REPLACE FUNCTION edax_depth(disc_count int, level int) RETURNS int
AS $$ SELECT (edax_search_params($1, $2)).depth $$ LANGUAGE sql IMMUTABLE;

CREATE OR REPLACE FUNCTION edax_confidence(disc_count int, level int) RETURNS int
AS $$ SELECT (edax_search_params($1, $2)).confidence $$ LANGUAGE sql IMMUTABLE;

-- Unlearned rows (level 0) are skipped: they store 0/0 rather than the formula's 0/100, and their
-- evaluation is never read.
DO $$
DECLARE
    checked bigint;
    mismatched bigint;
    sample text;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'boards' AND column_name IN ('depth', 'confidence')
    ) THEN
        RAISE NOTICE 'boards.depth/confidence are already gone; nothing to verify';
        RETURN;
    END IF;

    EXECUTE $q$
        SELECT count(*), count(*) FILTER (WHERE bad), string_agg(detail, E'\n') FILTER (WHERE bad)
        FROM (
            SELECT (depth, confidence) IS DISTINCT FROM
                       (edax_depth(disc_count, level), edax_confidence(disc_count, level)) AS bad,
                   format('  disc_count=%s level=%s stored=%s@%s%% derived=%s@%s%% rows=%s',
                          disc_count, level, depth, confidence,
                          edax_depth(disc_count, level), edax_confidence(disc_count, level), count(*)) AS detail
            FROM boards
            WHERE level > 0
            GROUP BY disc_count, level, depth, confidence
        ) grouped
    $q$ INTO checked, mismatched, sample;

    IF mismatched > 0 THEN
        RAISE EXCEPTION E'% of % (disc_count, level, depth, confidence) groups disagree with the level table:\n%',
            mismatched, checked, sample;
    END IF;

    RAISE NOTICE 'all % (disc_count, level) groups match the level table; dropping the columns is lossless', checked;
END;
$$;
