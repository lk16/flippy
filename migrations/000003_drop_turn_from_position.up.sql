-- position was 17 bytes: 8 of black discs, 8 of white discs, 1 turn byte. A position's evaluation
-- never depended on the turn byte -- edax only sees the discs of the player to move and those of
-- the opponent -- so othello.Board dropped it, and a position is now the 16 bytes player||opponent.
--
-- Converting a row is a byte shuffle: a black-to-move row keeps its halves, a white-to-move row
-- swaps them. That maps a board and its color swap onto one key, so such pairs merge, keeping the
-- deepest search of the two. Both rows describe the same edax problem, so they can only disagree
-- if one was searched deeper; scripts/verify_position_merge.sql proves that on the live book
-- BEFORE this runs, rather than assuming it.
--
-- Done as a table rewrite: DISTINCT ON needs the whole set anyway, and the new table comes out
-- compact instead of leaving the old rows as bloat.

CREATE TABLE boards_128 (
    position bytea NOT NULL PRIMARY KEY CHECK (octet_length(position) = 16),
    disc_count smallint NOT NULL,
    level smallint NOT NULL DEFAULT 0,
    score smallint NOT NULL DEFAULT 0
);

INSERT INTO boards_128 (position, disc_count, level, score)
SELECT DISTINCT ON (position) position, disc_count, level, score
FROM (
    SELECT
        CASE get_byte(position, 16)
            WHEN 0 THEN substring(position from 1 for 16)
            ELSE substring(position from 9 for 8) || substring(position from 1 for 8)
        END AS position,
        disc_count, level, score
    FROM boards
) converted
ORDER BY position, level DESC, score DESC;

DROP TABLE boards;

ALTER TABLE boards_128 RENAME TO boards;
ALTER TABLE boards RENAME CONSTRAINT boards_128_pkey TO boards_pkey;
ALTER TABLE boards RENAME CONSTRAINT boards_128_position_check TO boards_position_check;

CREATE INDEX idx_boards_disc_count_level ON boards (disc_count, level);
