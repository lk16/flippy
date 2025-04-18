from __future__ import annotations

import multiprocessing
import re
import subprocess
from multiprocessing import Queue
from typing import Optional

from flippy.config import get_edax_path, get_edax_verbose
from flippy.edax.types import EdaxEvaluation, EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.othello.position import NormalizedPosition, Position

TABLE_BORDER = (
    "------+-----+--------------+-------------+----------+---------------------"
)


def evaluate_non_blocking(
    request: EdaxRequest, recv_queue: Queue[EdaxResponse]
) -> None:
    if not request.positions:
        return

    process = EdaxProcessManager.get_instance().get_process(request.level)
    multiprocessing.Process(target=process.evaluate, args=(request, recv_queue)).start()


def evaluate_blocking(request: EdaxRequest) -> EdaxEvaluations:
    if not request.positions:
        return EdaxEvaluations()

    process = EdaxProcessManager.get_instance().get_process(request.level)
    evaluations = process.evaluate(request)

    # We always return evaluations in the blocking case.
    assert evaluations is not None

    return evaluations


class EdaxProcessManager:
    _instance: Optional[EdaxProcessManager] = None
    _process: Optional[EdaxProcess] = None
    _current_level: Optional[int] = None

    @classmethod
    def get_instance(cls) -> EdaxProcessManager:
        if cls._instance is None:
            cls._instance = EdaxProcessManager()
        return cls._instance

    def get_process(self, level: int) -> EdaxProcess:
        if self._process is None or self._current_level != level:
            if self._process is not None:
                self._process.close()
            self._process = EdaxProcess(level)
            self._current_level = level
        return self._process

    def close(self) -> None:
        if self._process is not None:
            self._process.close()
            self._process = None
            self._current_level = None


class EdaxProcess:
    def __init__(self, level: int) -> None:
        self.level = level
        self.edax_path = get_edax_path()
        self.verbose = get_edax_verbose()
        self.proc: Optional[subprocess.Popen[bytes]] = None
        self._start_process()

    def _start_process(self) -> None:
        command = f"{self.edax_path} -solve /dev/stdin -level {self.level} -verbose 3"
        cwd = self.edax_path.parent.parent

        self.proc = subprocess.Popen(
            command.split(),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            cwd=cwd,
        )

        assert self.proc.stdin
        assert self.proc.stdout

        if self.verbose:
            print(f"Started Edax process: {command}")
            print(f"CWD: {cwd}")

    def evaluate(
        self, request: EdaxRequest, send_queue: Optional[Queue[EdaxResponse]] = None
    ) -> Optional[EdaxEvaluations]:
        if not request.positions:
            return EdaxEvaluations() if send_queue is None else None

        assert self.proc and self.proc.stdin and self.proc.stdout

        # Make list such that we are sure order is maintained.
        positions_list = list(request.positions)

        proc_input = "".join(position.to_problem() for position in positions_list)
        self.proc.stdin.write(proc_input.encode())
        self.proc.stdin.flush()

        if self.verbose:
            print(f"Input: {proc_input}")

        lines: list[str] = []

        table_borders_seen = 0
        positions_index = 0

        evaluations = EdaxEvaluations()

        while positions_index < len(positions_list):
            raw_line = self.proc.stdout.readline()
            if raw_line == b"":
                # We have reached the end of the output.
                # We should never get here.
                raise ValueError("Unexpected end of output")
            line = raw_line.decode().rstrip()
            lines.append(line)

            if self.verbose:
                print(f"Output: {line}")

            if line == TABLE_BORDER:
                table_borders_seen += 1

            if table_borders_seen == 2:
                position = positions_list[positions_index]
                evaluation = self.__parse_output_line(lines[-3], position)
                evaluations[position] = evaluation

                table_borders_seen = 0
                positions_index += 1

        if send_queue is None:
            return evaluations
        else:
            message = EdaxResponse(request, evaluations)
            send_queue.put_nowait(message)
            return None

    def close(self) -> None:
        if self.proc is not None:
            self.proc.terminate()
            self.proc.wait()
            self.proc = None

    # TODO #26 write tests for Edax output parser
    def __parse_output_line(
        self, line: str, normalized: NormalizedPosition
    ) -> EdaxEvaluation:
        columns = re.sub(r"\s+", " ", line).strip().split(" ")
        score = int(columns[1].strip("<>"))

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
            level=self.level,
            confidence=confidence,
            score=score,
            best_moves=best_moves,
        )
