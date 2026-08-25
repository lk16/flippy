CREATE INDEX idx_boards_unlearned_first ON boards (((level > 0)), disc_count, level, position);
DROP INDEX idx_boards_unlearned;
