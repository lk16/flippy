from __future__ import annotations

import multiprocessing
import re
import subprocess
from multiprocessing import Queue
from typing import Optional

from flippy.config import get_edax_path, get_edax_verbose
from flippy.edax.types import EdaxEvaluation, EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.othello.position import NormalizedPosition, Position


def start_evaluation(request: EdaxRequest, recv_queue: Queue[EdaxResponse]) -> None:
    proc = EdaxProcess(request, recv_queue)

    if not request.positions:
        return

    multiprocessing.Process(target=proc.search).start()


def start_evaluation_sync(request: EdaxRequest) -> EdaxEvaluations:
    if not request.positions:
        return EdaxEvaluations()

    return EdaxProcess(request, Queue())._search_sync()


class EdaxProcess:
    def __init__(self, request: EdaxRequest, send_queue: Queue[EdaxResponse]) -> None:
        self.request = request
        self.send_queue = send_queue
        self.edax_path = get_edax_path()
        self.verbose = get_edax_verbose()

    def _search_sync(self) -> EdaxEvaluations:
        command = (
            f"{self.edax_path} -solve /dev/stdin -level {self.request.level} -verbose 3"
        )
        cwd = self.edax_path.parent.parent

        proc = subprocess.Popen(
            command.split(),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            cwd=cwd,
        )

        assert proc.stdin
        assert proc.stdout

        proc_input = "".join(
            position.to_problem() for position in self.request.positions
        )
        proc.stdin.write(proc_input.encode())
        proc.stdin.close()

        if self.verbose:
            print(f"Running command: {command}")
            print(f"CWD: {cwd}")
            print(f"Input: {proc_input}")

        lines: list[str] = []

        while True:
            raw_line = proc.stdout.readline()
            if raw_line == b"":
                break
            line = raw_line.decode()
            lines.append(line)

        if self.verbose:
            for line in lines:
                print(f"Output: {line.rstrip()}")

        evaluations = EdaxEvaluations()
        total_read_lines = 0
        for position in self.request.positions:
            remaining_lines = lines[total_read_lines:]
            evaluation, read_lines = self.__parse_output_lines(
                remaining_lines, position
            )
            total_read_lines += read_lines
            evaluations[position] = evaluation

        return evaluations

    def search(self) -> None:
        # Catching user interrupt prevents the subproces stacktrace cluttering the output.
        try:
            evaluations = self._search_sync()
            message = EdaxResponse(self.request, evaluations)
            self.send_queue.put_nowait(message)
        except KeyboardInterrupt:
            pass

    # TODO #26 write tests for Edax output parser
    def __parse_output_lines(
        self, lines: list[str], normalized: NormalizedPosition
    ) -> tuple[EdaxEvaluation, int]:
        evaluation: Optional[EdaxEvaluation] = None

        for read_lines, line in enumerate(lines):
            if line.startswith("*** problem") and read_lines > 2:
                break

            line_evaluation = self.__parse_output_line(line, normalized)
            if line_evaluation is not None:
                evaluation = line_evaluation

        assert evaluation
        return evaluation, read_lines

    # TODO #26 write tests for Edax output parser
    def __parse_output_line(
        self, line: str, normalized: NormalizedPosition
    ) -> Optional[EdaxEvaluation]:
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
        best_moves = Position.fields_to_indexes(best_fields)
        depth = int(columns[0].split("@")[0])

        if "@" not in columns[0]:
            confidence = 100
        else:
            confidence = int(columns[0].split("@")[1].split("%")[0])

        return EdaxEvaluation(
            position=normalized.to_position(),
            depth=depth,
            level=self.request.level,
            confidence=confidence,
            score=score,
            best_moves=best_moves,
        )
