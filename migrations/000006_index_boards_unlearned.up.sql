-- The job candidate sweep no longer pages through one ordering that puts unlearned rows first:
-- it scans the unlearned rows on their own, from the start, before falling back to the
-- partially-learned sweep. A partial index serves that scan out of far fewer pages than the
-- expression index it replaces, which carried every row in the book.
-- idx_boards_disc_count_level_position covers the partially-learned sweep.
CREATE INDEX idx_boards_unlearned ON boards (disc_count, position) WHERE level = 0;
DROP INDEX idx_boards_unlearned_first;
