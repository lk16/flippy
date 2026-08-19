-- Verifies that the rows migration 000003 merges really are the same position, i.e. that dropping
-- the turn byte from boards.position loses no evaluation. Run it against the live book BEFORE
-- migrating:
--
--   psql "$FLIPPY_POSTGRES_URL" -f scripts/verify_position_merge.sql
--
-- Two 17-byte rows collapse onto one 16-byte position exactly when one is the other with the colors
-- swapped: black||white||black-to-move and white||black||white-to-move both mean "these discs are
-- the mover's, those are the opponent's". edax evaluates both identically, so a pair may only
-- differ where one was searched deeper. This raises an exception listing any pair that disagrees at
-- the same level -- the one case where the migration's "keep the deepest" would silently discard a
-- real disagreement -- and otherwise reports how many rows the merge removes.

DO $$
DECLARE
    merged bigint;
    conflicts bigint;
    sample text;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM boards WHERE octet_length(position) = 17) THEN
        RAISE NOTICE 'no 17-byte positions left; migration 000003 has already run';
        RETURN;
    END IF;

    CREATE TEMPORARY TABLE converted ON COMMIT DROP AS
    SELECT
        CASE get_byte(position, 16)
            WHEN 0 THEN substring(position from 1 for 16)
            ELSE substring(position from 9 for 8) || substring(position from 1 for 8)
        END AS position,
        level, score
    FROM boards;

    SELECT count(*) - count(DISTINCT position) INTO merged FROM converted;

    SELECT count(*) INTO conflicts
    FROM (
        SELECT 1 FROM converted WHERE level > 0 GROUP BY position, level HAVING count(DISTINCT score) > 1
    ) bad;

    IF conflicts > 0 THEN
        SELECT string_agg(detail, E'\n') INTO sample
        FROM (
            SELECT format('  position=%s level=%s scores=%s', encode(position, 'hex'), level,
                          string_agg(DISTINCT score::text, '/')) AS detail
            FROM converted
            WHERE level > 0
            GROUP BY position, level
            HAVING count(DISTINCT score) > 1
            LIMIT 20
        ) worst;

        RAISE EXCEPTION E'% color-swapped pairs disagree on score at the same level:\n%', conflicts, sample;
    END IF;

    RAISE NOTICE 'merge is lossless: % of % rows are color-swapped duplicates of another row',
        merged, (SELECT count(*) FROM converted);
END;
$$;
