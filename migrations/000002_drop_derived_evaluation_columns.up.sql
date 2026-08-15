-- depth and confidence follow from (disc_count, level) via edax's search_global_init, ported as
-- edax.SearchParams, so storing them per row costs 4 bytes per board for no information. Nothing
-- reads them in SQL any more; they are computed on read instead.
-- scripts/verify_derived_columns.sql checks an existing book agrees with the mapping before this
-- runs, so the drop is provably lossless rather than assumed to be.
ALTER TABLE boards
    DROP COLUMN depth,
    DROP COLUMN confidence;
