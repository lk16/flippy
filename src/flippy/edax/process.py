from pathlib import Path
import re
import select
import subprocess
import time
from typing import Optional
from flippy.config import config
from flippy.othello.board import Board


class NoResponseLineError(Exception):
    pass


class EdaxProcess:
    def __init__(self, board: Board) -> None:
        self.edax_path = config.edax_path()
        self.board = board
        self.process: subprocess.Popen[bytes]
        self.last_eval: Optional[int] = None
        self.last_eval_update = 0.0

    def _send(self, command: str) -> None:
        assert self.process.stdin
        self.process.stdin.write(f"{command}\n".encode())
        self.process.stdin.flush()

    def _recv(self, wait: bool = True) -> str:
        if wait:
            timeout = None
        else:
            timeout = 0.1

        ready, _, _ = select.select([self.process.stdout], [], [], timeout)
        if not ready:
            raise NoResponseLineError

        assert self.process.stdout
        response: str = self.process.stdout.readline().decode().strip()
        return response

    def _update_last_eval(self) -> None:
        while True:
            try:
                response = self._recv(wait=False)
            except NoResponseLineError:
                break

            columns = re.sub(r"\s+", " ", response).split(" ")
            eval = int(columns[1])
            self.last_eval = eval // -100

    def get_last_eval(self) -> Optional[int]:
        now = time.time()

        if now - self.last_eval_update > 0.1:
            self._update_last_eval()
            self.last_eval_update = now

        return self.last_eval

    def start(self) -> None:
        self.process = subprocess.Popen(
            [self.edax_path, "-xboard"],
            stdout=subprocess.PIPE,
            stdin=subprocess.PIPE,
            cwd=Path(self.edax_path).parent.parent,
        )

        # Print heuristic and best line.
        self._send("post")

        # Use 1 core.
        self._send("cores 1")

        # Configure board to search.
        self._send("setboard " + self.board.to_fen())

        # Start analyzing.
        self._send("analyze")

        # Discard first response.
        self._recv()

    def kill(self) -> None:
        self.process.kill()
