-- Restores the columns empty: their values follow from (disc_count, level), so nothing is lost by
-- not storing them and nothing can be restored that isn't recomputable. To fill them back in, run
-- scripts/verify_derived_columns.sql (it defines edax_depth/edax_confidence) and then:
--   UPDATE boards SET depth = edax_depth(disc_count, level),
--                     confidence = edax_confidence(disc_count, level) WHERE level > 0;
ALTER TABLE boards
    ADD COLUMN depth smallint NOT NULL DEFAULT 0,
    ADD COLUMN confidence smallint NOT NULL DEFAULT 0;
