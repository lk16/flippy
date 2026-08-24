-- The job candidate sweep hands unlearned rows out before partially learned ones, so it now pages
-- through ((level > 0), disc_count, level, position) with a row comparison. Only an index on that
-- same expression keeps it an index range scan rather than a sort over the whole book;
-- idx_boards_disc_count_level_position stays for the queries that page by disc count alone.
CREATE INDEX idx_boards_unlearned_first ON boards (((level > 0)), disc_count, level, position);
