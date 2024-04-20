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


class EdaxProcess:
    def __init__(self, request: EdaxRequest, send_queue: Queue[EdaxResponse]) -> None:
        self.request = request
        self.send_queue = send_queue
        self.edax_path = config.edax_path()

        if isinstance(request.task, Board):
            boards = request.task.get_children()
        else:
            boards = request.task.get_all_children()

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

    def search_sync(self) -> EdaxEvaluations:
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

        evaluations: dict[Board, EdaxEvaluation] = {}
        total_read_lines = 0
        for board in self.searchable_boards:
            remaining_lines = lines[total_read_lines:]
            evaluation, read_lines = self.__parse_output_lines(remaining_lines, board)

            total_read_lines += read_lines
            assert board.is_valid_move(evaluation.best_move)
            evaluations[board] = evaluation

        return EdaxEvaluations(evaluations)

    def search(self) -> None:
        evaluations = self.search_sync()
        message = EdaxResponse(self.request, evaluations)
        self.send_queue.put_nowait(message)

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

    # TODO#26 write tests for Edax output parser
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
