-- Restores the 17-byte position: 8 bytes of black discs, 8 of white discs, 1 turn byte.
--
-- Lossy, but only in a way the book doesn't care about: which color was to move is gone, so every
-- row comes back as black to move (player discs become black's, opponent discs white's, turn byte
-- 0). That is the same position with the same score, and it is still normalized, so a book rolled
-- back this way keeps working; only the bytes of rows that were once stored white-to-move differ.

CREATE TABLE boards_17 (
    position bytea NOT NULL PRIMARY KEY,
    disc_count smallint NOT NULL,
    level smallint NOT NULL DEFAULT 0,
    score smallint NOT NULL DEFAULT 0
);

INSERT INTO boards_17 (position, disc_count, level, score)
SELECT position || '\x00'::bytea, disc_count, level, score
FROM boards;

DROP TABLE boards;

ALTER TABLE boards_17 RENAME TO boards;
ALTER TABLE boards RENAME CONSTRAINT boards_17_pkey TO boards_pkey;

CREATE INDEX idx_boards_disc_count_level ON boards (disc_count, level);
