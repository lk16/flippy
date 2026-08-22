CREATE INDEX idx_boards_disc_count_level ON boards (disc_count, level);
DROP INDEX idx_boards_disc_count_level_position;
