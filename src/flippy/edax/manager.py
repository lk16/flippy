from __future__ import annotations
from copy import copy
import re
import subprocess
from flippy.config import config
from multiprocessing import Queue
import queue
from typing import Any, Optional

from flippy.othello.board import EMPTY, PASS_MOVE, Board


class EdaxEvaluation:
    def __init__(self, depth: str, score: int, best_move: int) -> None:
        self.depth = depth
        self.score = score
        self.best_move = best_move


class EdaxEvaluations:
    def __init__(self, values: dict[Board, EdaxEvaluation]) -> None:
        self.values = values
        self._validate()

    def _validate(self) -> None:
        for board, eval in self.values.items():
            assert board.is_normalized()
            assert board.is_valid_move(eval.best_move)

    def lookup(self, board: Board) -> EdaxEvaluation:
        if board.is_game_end():
            return self._lookup_game_end(board)
        if not board.has_moves():
            return self._lookup_passed(board)

        key, rotation = board.normalized()
        value = copy(self.values[key])
        value.best_move = Board.unrotate_move(value.best_move, rotation)
        return value

    def _lookup_game_end(self, board: Board) -> EdaxEvaluation:
        empties = str(board.count(EMPTY))
        score = board.get_final_score()
        return EdaxEvaluation(empties, score, PASS_MOVE)

    def _lookup_passed(self, board: Board) -> EdaxEvaluation:
        passed = board.pass_move()
        value = copy(self.lookup(passed))

        value.best_move = PASS_MOVE
        value.score *= -1
        return value

    def update(self, other: EdaxEvaluations) -> None:
        # TODO worry about losing items with lower search depth
        self.values.update(other.values)

    def has_all_children(self, board: Board) -> bool:
        for move in board.get_moves_as_set():
            key = board.do_normalized_move(move)
            if key not in self.values:
                return False
        return True


class EdaxManager:
    def __init__(
        self,
        send_queue: "Queue[EdaxEvaluations]",
        recv_queue: "Queue[tuple[str, Any]]",
    ):
        self.send_queue = send_queue
        self.recv_queue = recv_queue
        self.edax_path = config.edax_path()
        self.loop_running = False

    def loop(self) -> None:
        self.loop_running = True

        while True:
            try:
                message = self.__get_last_message()
            except queue.Empty:
                continue

            if message[0] == "set_board":
                board = message[1]

                for depth in range(4, 24, 2):
                    evaluations = self.__evaluate(board.get_children(), depth)
                    self.send_queue.put(evaluations)
                    if not self.recv_queue.empty():
                        break
            else:
                print(f"Unhandled message kind {message[0]}")

    def evaluate(self, boards: list[Board], level: int) -> EdaxEvaluations:
        if self.loop_running:
            raise ValueError(
                "Cannot call evaluate() when loop is running! Use queues instead."
            )

        return self.__evaluate(boards, level)

    def __get_last_message(self) -> tuple[str, Any]:
        last_message: Optional[tuple[str, Any]] = None
        while True:
            try:
                message = self.recv_queue.get_nowait()
            except queue.Empty:
                break
            else:
                last_message = message

        if not last_message:
            raise queue.Empty
        return last_message

    def __evaluate(self, boards: list[Board], level: int) -> EdaxEvaluations:
        normalized_boards: set[Board] = set()

        for board in boards:
            if board.is_game_end():
                continue
            elif not board.has_moves():
                passed = board.pass_move()
                normalized_boards.add(passed.normalized()[0])
            else:
                normalized_board = board.normalized()[0]
                normalized_boards.add(normalized_board)

        proc = subprocess.Popen(
            [
                self.edax_path,
                "-solve",
                "/dev/stdin",
                "-level",
                str(level),
                "-verbose",
                "3",
            ],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            cwd=self.edax_path.parent.parent,
        )

        assert proc.stdin
        assert proc.stdout

        proc_input = "".join(board.to_problem() for board in normalized_boards)
        proc.stdin.write(proc_input.encode())
        proc.stdin.close()

        lines: list[str] = []

        while True:
            raw_line = proc.stdout.readline()
            if raw_line == b"":
                break
            line = raw_line.decode()

            # TODO add verbose option
            # print(line, end="")

            lines.append(line)

        evaluations: dict[Board, EdaxEvaluation] = {}
        total_read_lines = 0
        for board in normalized_boards:
            remaining_lines = lines[total_read_lines:]
            evaluation, read_lines = self.__parse_output_lines(remaining_lines, board)

            total_read_lines += read_lines
            assert board.is_valid_move(evaluation.best_move)
            evaluations[board] = evaluation

        return EdaxEvaluations(evaluations)

    # TODO write tests
    def __parse_output_lines(
        self, lines: list[str], board: Board
    ) -> tuple[EdaxEvaluation, int]:
        evaluation: Optional[EdaxEvaluation] = None

        for read_lines, line in enumerate(lines):
            if line.startswith("*** problem") and read_lines > 2:
                break

            line_evaluation = self.__parse_output_line(line, board)
            if line_evaluation is not None:
                evaluation = line_evaluation

        assert evaluation
        return evaluation, read_lines

    # TODO write tests
    def __parse_output_line(self, line: str, board: Board) -> Optional[EdaxEvaluation]:
        if (
            line == "\n"
            or "positions;" in line
            or "/dev/stdin" in line
            or "-----" in line
        ):
            return None

        columns = re.sub(r"\s+", " ", line).strip().split(" ")

        try:
            score = int(columns[1].strip("<>"))
        except ValueError:
            return None

        best_field = line[53:].split(" ")[0]
        best_move = Board.field_to_index(best_field)
        depth = columns[0]

        return EdaxEvaluation(depth, score, best_move)
