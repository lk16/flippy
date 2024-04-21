from __future__ import annotations

import multiprocessing
import re
import subprocess
from multiprocessing import Queue
from typing import Optional

from flippy.config import config
from flippy.edax.types import EdaxEvaluation, EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.othello.board import Board


def start_evaluation(request: EdaxRequest, recv_queue: Queue[EdaxResponse]) -> None:
    proc = EdaxProcess(request, recv_queue)
    multiprocessing.Process(target=proc.search).start()


def start_evaluation_sync(request: EdaxRequest) -> EdaxEvaluations:
    return EdaxProcess(request, Queue())._search_sync()


class EdaxProcess:
    def __init__(self, request: EdaxRequest, send_queue: Queue[EdaxResponse]) -> None:
        self.request = request
        self.send_queue = send_queue
        self.edax_path = config.edax_path()

        if isinstance(request.task, Board):
            boards = request.task.get_children()
        else:
            boards = request.task.boards

        searchable: list[Board] = []

        for board in boards:
            if board.is_game_end():
                # Discard if the game is over.
                continue
            elif not board.has_moves():
                # Pass if there are no moves, but opponent has moves.
                # Edax crashes when asked to solve a position without moves.
                passed = board.pass_move()
                searchable.append(passed.normalized()[0])
            else:
                normalized_board = board.normalized()[0]
                searchable.append(normalized_board)

        self.searchable_boards = set(searchable)

    def _search_sync(self) -> EdaxEvaluations:
        proc = subprocess.Popen(
            f"{self.edax_path} -solve /dev/stdin -level {self.request.level} -verbose 3".split(),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            cwd=self.edax_path.parent.parent,
        )

        assert proc.stdin
        assert proc.stdout

        proc_input = "".join(board.to_problem() for board in self.searchable_boards)
        proc.stdin.write(proc_input.encode())
        proc.stdin.close()

        lines: list[str] = []

        while True:
            raw_line = proc.stdout.readline()
            if raw_line == b"":
                break
            line = raw_line.decode()
            lines.append(line)

        evaluations = EdaxEvaluations()
        total_read_lines = 0
        for board in self.searchable_boards:
            remaining_lines = lines[total_read_lines:]
            evaluation, read_lines = self.__parse_output_lines(remaining_lines, board)
            total_read_lines += read_lines
            evaluations.add(board, evaluation)

            best_child, child_evaluation = self._get_child_evaluation(board, evaluation)
            evaluations.add(best_child, child_evaluation)

        return evaluations

    def search(self) -> None:
        evaluations = self._search_sync()
        message = EdaxResponse(self.request, evaluations)
        self.send_queue.put_nowait(message)

    def _get_child_evaluation(
        self, board: Board, evaluation: EdaxEvaluation
    ) -> tuple[Board, EdaxEvaluation]:
        best_child, child_rotation = board.do_move(
            evaluation.best_moves[0]
        ).normalized()

        best_child_moves = [
            Board.rotate_move(move, child_rotation)
            for move in evaluation.best_moves[1:]
        ]

        child_evaluation = EdaxEvaluation(
            best_child,
            evaluation.depth - 1,
            evaluation.confidence,
            -evaluation.score,
            best_child_moves,
        )

        return best_child, child_evaluation

    # TODO #26 write tests for Edax output parser
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

    # TODO #26 write tests for Edax output parser
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

        best_fields = line[53:].strip().split(" ")
        best_moves = Board.fields_to_indexes(best_fields)
        depth = int(columns[0].split("@")[0])

        if "@" not in columns[0]:
            confidence = 100
        else:
            confidence = int(columns[0].split("@")[1].split("%")[0])

        return EdaxEvaluation(board, depth, confidence, score, best_moves)
