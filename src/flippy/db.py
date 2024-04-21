import psycopg2
from math import ceil

from flippy.config import config
from flippy.edax.types import EdaxEvaluation, EdaxEvaluations
from flippy.othello.position import Position

# Minimum edax search level for an evaluation to be potentially saved in DB.
MIN_LEARN_LEVEL = 16

# Minimum edax search level when ran from user interface
MIN_UI_SEARCH_LEVEL = 8

# Maximum edax search level when ran from user interface
MAX_UI_SEARCH_LEVEL = 32

# Maxmium number of discs for a board to be potentially saved in DB.
MAX_SAVABLE_DISCS = 35


def is_savable_evaluation(evaluation: EdaxEvaluation) -> bool:
    """
    Checks whether an evaluation qualifies to be saved in the DB.
    """
    return evaluation.depth >= MIN_LEARN_LEVEL and is_savable_position(
        evaluation.position
    )


def is_savable_position(position: Position) -> bool:
    """
    Checks whether a position qualifies to be saved in the DB.
    """
    return position.has_moves() and position.count_discs() <= MAX_SAVABLE_DISCS


class PositionNotFound(Exception):
    pass


class DB:
    def __init__(self) -> None:
        dsn = config.get_db_dsn()
        self.conn = psycopg2.connect(dsn)

    def update(self, evaluations: EdaxEvaluations) -> None:
        for evaluation in evaluations.values.values():
            self.save(evaluation)

    def lookup_position(self, position: Position) -> EdaxEvaluation:
        evaluations = self.lookup_positions({position})

        try:
            return evaluations.lookup(position)
        except KeyError as e:
            raise PositionNotFound from e

    def lookup_positions(self, positions: set[Position]) -> EdaxEvaluations:
        evaluations = EdaxEvaluations()

        query_positions: list[Position] = []

        for position in positions:
            normalized = position.normalized()

            if normalized.is_game_end():
                try:
                    evaluation = self._lookup_game_end(normalized)
                except PositionNotFound:
                    pass
                else:
                    evaluations.add(normalized, evaluation)
                continue

            if not position.has_moves():
                try:
                    evaluation = self._lookup_passed(normalized)
                except PositionNotFound:
                    pass
                else:
                    evaluations.add(normalized, evaluation)
                continue

            else:
                query_positions.append(normalized)

        chunk_size = 1000
        for chunk_id in range(ceil(len(query_positions) / chunk_size)):
            chunk_start = chunk_size * chunk_id
            chunk_end = chunk_size * (chunk_id + 1)
            chunk = query_positions[chunk_start:chunk_end]

            query = """
            SELECT me, opp, level, depth, confidence, score, best_moves
            FROM openings
            WHERE (me, opp)
            IN (
            """

            query += ",".join(["(%s, %s)"] * len(chunk)) + ");"

            params: list[bytes] = []
            for position in chunk:
                me, opp = position.to_bytes()
                params += [me, opp]

            cursor = self.conn.cursor()
            cursor.execute(query, params)

            rows: list[
                tuple[memoryview, memoryview, int, int, int, int, list[int]]
            ] = cursor.fetchall()
            for me, opp, level, depth, confidence, score, best_moves in rows:
                position = Position.from_bytes(bytes(me), bytes(opp))
                evaluation = EdaxEvaluation(
                    position=position,
                    level=level,
                    depth=depth,
                    confidence=confidence,
                    score=score,
                    best_moves=best_moves,
                )
                evaluations.add(position, evaluation)

        return evaluations

    def _lookup_game_end(self, position: Position) -> EdaxEvaluation:
        return EdaxEvaluation(
            position=position,
            depth=position.count_empties(),
            level=position.count_empties(),
            confidence=100,
            score=position.get_final_score(),
            best_moves=[],
        )

    def _lookup_passed(self, position: Position) -> EdaxEvaluation:
        passed = position.pass_move()
        return self.lookup_position(passed).pass_move()

    def save(self, evaluation: EdaxEvaluation) -> None:
        if not is_savable_evaluation(evaluation):
            return

        inserted = self._insert(evaluation)

        if not inserted:
            self._update(evaluation)

    def _insert(self, evaluation: EdaxEvaluation) -> int:
        """
        Tries to insert, returns True if succcess, False if conflict occured.
        """

        evaluation._validate()

        cursor = self.conn.cursor()
        me, opp = evaluation.position.to_bytes()

        assert evaluation.position.is_normalized()

        disc_count = evaluation.position.count_discs()
        priority = 3 * evaluation.level + disc_count

        params = (
            me,
            opp,
            evaluation.position.count_discs(),
            evaluation.level,
            evaluation.depth,
            evaluation.confidence,
            evaluation.score,
            priority,
            evaluation.best_moves,
        )

        query = """
        INSERT INTO openings (me, opp, disc_count, level, depth, confidence, score, learn_priority, best_moves)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (me, opp)
        DO NOTHING;
        """
        cursor.execute(query, params)
        self.conn.commit()

        return cursor.rowcount == 1

    def _update(self, evaluation: EdaxEvaluation) -> None:
        me, opp = evaluation.position.to_bytes()

        query = """
        UPDATE openings
        SET disc_count=%s, level=%s, depth=%s, confidence=%s, score=%s, learn_priority=%s, best_moves=%s
        WHERE (me, opp) = (%s, %s) AND level < %s;
        """

        disc_count = evaluation.position.count_discs()
        priority = 3 * evaluation.level + disc_count

        params = (
            evaluation.position.count_discs(),
            evaluation.level,
            evaluation.depth,
            evaluation.confidence,
            evaluation.score,
            priority,
            evaluation.best_moves,
            me,
            opp,
            evaluation.level,
        )

        cursor = self.conn.cursor()
        cursor.execute(query, params)
        self.conn.commit()

    def _get_stats(self) -> list[tuple[int, int, int]]:
        query = """
        SELECT disc_count, level, COUNT(*)
        FROM openings
        GROUP BY disc_count, level;
        """
        cursor = self.conn.cursor()
        cursor.execute(query)
        return cursor.fetchall()

    def get_learning_boards_below_level(
        self, count: int, level: int
    ) -> list[tuple[Position, int]]:
        cursor = self.conn.cursor()

        query = """
        SELECT me, opp, depth
        FROM openings
        WHERE depth < %s
        ORDER BY disc_count
        LIMIT %s;
        """

        cursor.execute(query, (level, count))
        rows: list[tuple[bytes, bytes, int]] = cursor.fetchall()
        return [(Position.from_bytes(me, opp), depth) for me, opp, depth in rows]

    def get_learning_boards(self, count: int) -> list[tuple[Position, int]]:
        cursor = self.conn.cursor()

        query = """
        SELECT me, opp, level
        FROM openings
        ORDER BY learn_priority
        LIMIT %s;
        """

        cursor.execute(query, (count,))
        rows: list[tuple[bytes, bytes, int]] = cursor.fetchall()
        return [(Position.from_bytes(me, opp), depth) for me, opp, depth in rows]

    def print_stats(self) -> None:
        stats = self._get_stats()
        table: dict[int, dict[int, int]] = {}

        for discs, level, count in stats:
            if discs not in table:
                table[discs] = {}

            if level not in table[discs]:
                table[discs][level] = count

        levels = set(row[1] for row in stats)

        print("   level: " + " ".join(f"{level:>5}" for level in sorted(levels)))
        print("----------" + "-".join("-----" for _ in levels))

        for discs in sorted(table.keys()):
            print(f"{discs:>2} discs: ", end="")
            for level in sorted(levels):
                if level not in table[discs]:
                    print("      ", end="")
                else:
                    print(f"{table[discs][level]:>5} ", end="")
            print()
