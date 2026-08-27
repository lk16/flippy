DROP TRIGGER board_stats_truncate ON boards;
DROP TRIGGER board_stats_delete ON boards;
DROP TRIGGER board_stats_update ON boards;
DROP TRIGGER board_stats_insert ON boards;
DROP FUNCTION board_stats_truncate();
DROP FUNCTION board_stats_apply();
DROP TABLE board_stats;
