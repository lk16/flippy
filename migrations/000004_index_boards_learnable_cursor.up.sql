-- The job candidate sweep pages through (disc_count, level, position) with a row comparison, which
-- only stays an index range scan with position in the index. The old two-column index is a prefix
-- of this one, so every query that used it is still covered.
CREATE INDEX idx_boards_disc_count_level_position ON boards (disc_count, level, position);
DROP INDEX idx_boards_disc_count_level;
