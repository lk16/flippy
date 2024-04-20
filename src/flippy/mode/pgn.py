import multiprocessing
import pygame
import queue
import tkinter as tk
from multiprocessing import Queue
from pathlib import Path
from pygame.event import Event
from tkinter import filedialog
from typing import Any, Optional

from flippy.arguments import Arguments
from flippy.edax.evaluations import EdaxEvaluations
from flippy.edax.manager import EdaxManager
from flippy.mode.base import BaseMode
from flippy.othello.board import Board
from flippy.othello.game import Game


class PGNMode(BaseMode):
    def __init__(self, args: Arguments) -> None:
        self.args = args.pgn
        self.game: Optional[Game] = None
        self.moves_done = 0
        self.send_queue: Queue[tuple[Any, ...]] = Queue()
        self.recv_queue: Queue[EdaxEvaluations] = Queue()
        self.all_evaluations = EdaxEvaluations({})
        self.edax_manager = EdaxManager(self.recv_queue, self.send_queue)

        multiprocessing.Process(target=self.edax_manager.loop).start()

        if self.args.pgn_file:
            self.game = Game.from_pgn(self.args.pgn_file)
            self.send_queue.put_nowait(("set_game", self.game))

    def on_frame(self, event: Event) -> None:
        if self.game:
            return

        self.select_pgn_file()

    def on_event(self, event: Event) -> None:
        if not (
            event.type == pygame.MOUSEBUTTONDOWN and event.button == pygame.BUTTON_RIGHT
        ):
            return

        if self.moves_done != 0:
            self.moves_done -= 1

    def on_move(self, move: int) -> None:
        if self.game is None:
            return

        if self.moves_done != len(self.game.boards) - 1:
            self.moves_done += 1

    def get_board(self) -> Board:
        if self.game is None:
            return Board.start()
        return self.game.boards[self.moves_done]

    def get_played_move(self) -> Optional[int]:
        if self.game is None or self.moves_done >= len(self.game.moves):
            return None

        return self.game.moves[self.moves_done]

    def select_pgn_file(self) -> Optional[Path]:
        root = tk.Tk()
        root.withdraw()  # Hide the main window

        file_path = filedialog.askopenfilename(
            title="Select PGN File",
            filetypes=[("PGN files", "*.pgn"), ("All files", "*.*")],
            initialdir="./pgn",
        )

        if not file_path:
            return None

        pgn_file = Path(file_path)

        self.game = Game.from_pgn(pgn_file)
        self.moves_done = 0

        self.send_queue.put_nowait(("set_game", self.game))

        return pgn_file

    def _process_recv_messages(self) -> None:
        while True:
            try:
                evaluations = self.recv_queue.get_nowait()
            except queue.Empty:
                break
            else:
                self.all_evaluations.update(evaluations)

    def get_ui_details(self) -> dict[str, Any]:
        self._process_recv_messages()

        evaluations: dict[int, int] = {}
        board = self.get_board()

        for move in board.get_moves_as_set():
            child = board.do_move(move)

            try:
                evaluation = self.all_evaluations.lookup(child)
            except KeyError:
                continue

            evaluations[move] = -evaluation.score

        ui_details: dict[str, Any] = {"evaluations": evaluations}

        played_move = self.get_played_move()
        if played_move is not None:
            ui_details["played_move"] = played_move

        return ui_details
