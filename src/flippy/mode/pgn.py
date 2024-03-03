from pathlib import Path
from typing import Optional

import pygame
from flippy.board import Board
from pygame.event import Event
from flippy.mode.base import BaseMode

from flippy.game import Game

import tkinter as tk
from tkinter import filedialog


class PGNMode(BaseMode):
    def __init__(self) -> None:
        self.game: Optional[Game] = None
        self.moves_done = 0

    def on_frame(self, event: Event) -> None:
        if self.game:
            return

        game_path = self.select_pgn_file()
        if not game_path:
            return

        self.game = Game.from_pgn(game_path)
        self.moves_done = len(self.game.moves)

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

        return Path(file_path)
