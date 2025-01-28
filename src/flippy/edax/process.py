from __future__ import annotations

import multiprocessing
import re
import subprocess
from multiprocessing import Queue

from flippy.config import get_edax_path, get_edax_verbose
from flippy.edax.types import EdaxEvaluation, EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.othello.position import Position


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
        # TODO make request argument of search()
        self.request = request
        self.send_queue = send_queue
        self.edax_path = get_edax_path()
        self.verbose = get_edax_verbose()

        searchable: set[Position] = set()

        # TODO move validation out of __init__()
        for position in request.positions:
            if position.is_game_end():
                # Discard if the game is over.
                continue
            elif not position.has_moves():
                # Pass if there are no moves, but opponent has moves.
                # Edax crashes when asked to solve a position without moves.
                passed = position.pass_move()
                searchable.add(passed.normalized())
            else:
                searchable.add(position.normalized())

        self.searchable_positions = searchable

    # TODO have one search function:
    # - make sync function on top of this file handle waiting for the process to finish
    # - make async function on top of this file just call this in sub process
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
        evaluations = EdaxEvaluations()

        if self.verbose:
            print(f"Running command: {command}")
            print(f"CWD: {cwd}")

        for board in self.searchable_positions:
            problem = board.to_problem()

            print(f"Input: {problem}")
            proc.stdin.write(problem.encode())
            proc.stdin.flush()

            lines: list[str] = []
            dash_prefixes_found = 0

            while True:
                line = proc.stdout.readline().decode()
                print(f"Output: {line.rstrip()}")
                lines.append(line)

                if line.startswith("-----"):
                    dash_prefixes_found += 1

                if dash_prefixes_found == 2:
                    break

            eval_line = lines[-3]
            evaluation = self.__parse_output_line(eval_line, board)
            evaluations.add(board, evaluation)

        proc.kill()

        return evaluations

    def search(self) -> None:
        # Catching user interrupt prevents the subproces stacktrace cluttering the output.
        try:
            evaluations = self._search_sync()
            message = EdaxResponse(self.request, evaluations)
            self.send_queue.put_nowait(message)
        except KeyboardInterrupt:
            pass

    # TODO write tests for Edax output parser
    # TODO this fails for 100% confidence
    def __parse_output_line(self, line: str, position: Position) -> EdaxEvaluation:
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
            position=position,
            depth=depth,
            level=self.request.level,
            confidence=confidence,
            score=score,
            best_moves=best_moves,
        )
