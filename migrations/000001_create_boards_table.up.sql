CREATE TABLE boards (
    position bytea NOT NULL PRIMARY KEY,
    disc_count smallint NOT NULL,
    level smallint NOT NULL DEFAULT 0,
    depth smallint NOT NULL DEFAULT 0,
    confidence smallint NOT NULL DEFAULT 0,
    score smallint NOT NULL DEFAULT 0
);

CREATE INDEX idx_boards_disc_count_level ON boards (disc_count, level);
