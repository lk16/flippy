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
from flippy.edax.process import start_board_evaluation, start_game_evaluation
from flippy.mode.base import BaseMode
from flippy.othello.board import Board, InvalidMove
from flippy.othello.game import Game


class PGNMode(BaseMode):
    def __init__(self, args: Arguments) -> None:
        self.args = args.pgn
        self.game: Optional[Game] = None
        self.moves_done = 0
        self.alternative_moves: list[Board] = []
        self.recv_queue: Queue[tuple[int, Board | Game, EdaxEvaluations]] = Queue()
        self.all_evaluations = EdaxEvaluations({})

        if self.args.pgn_file:
            self.game = Game.from_pgn(self.args.pgn_file)
            start_game_evaluation(2, self.game, self.recv_queue)

    def on_frame(self, event: Event) -> None:
        if self.game:
            return

        self.select_pgn_file()

    def on_event(self, event: Event) -> None:
        if event.type == pygame.KEYDOWN:
            if event.key == pygame.K_RIGHT:
                self.show_next_position()
            elif event.key == pygame.K_LEFT:
                self.show_prev_position()

        if event.type == pygame.MOUSEBUTTONDOWN and event.button == pygame.BUTTON_RIGHT:
            self.show_prev_position()

    def on_move(self, move: int) -> None:
        if not self.game:
            return

        try:
            child = self.get_board().do_move(move)
        except InvalidMove:
            return

        try:
            next_board = self.game.boards[self.moves_done + 1]
        except IndexError:
            pass
        else:
            if next_board == child:
                # User clicked on square that was actually played in game.
                # We do not handle it as alternative move.
                self.moves_done += 1
                return

        if not child.has_moves() and child.pass_move().has_moves():
            # Opponent ran out of moves, but the game is not over.
            child = child.pass_move()

        self.alternative_moves.append(child)
        start_board_evaluation(2, child, self.recv_queue)

    def show_next_position(self) -> None:
        if self.game is None:
            return

        if self.alternative_moves:
            return

        max_moves_done = len(self.game.boards) - 1
        self.moves_done = min(self.moves_done + 1, max_moves_done)

    def show_prev_position(self) -> None:
        if self.game is None:
            return

        if self.alternative_moves:
            self.alternative_moves.pop()
            return

        self.moves_done = max(self.moves_done - 1, 0)

    def get_board(self) -> Board:
        if self.game is None:
            return Board.start()

        if self.alternative_moves:
            return self.alternative_moves[-1]

        return self.game.boards[self.moves_done]

    def get_played_move(self) -> Optional[int]:
        if (
            self.game is None
            or self.moves_done >= len(self.game.moves)
            or self.alternative_moves
        ):
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

        start_game_evaluation(2, self.game, self.recv_queue)
        return pgn_file

    def _process_recv_messages(self) -> None:
        while True:
            try:
                message = self.recv_queue.get_nowait()
            except queue.Empty:
                break

            self._process_recv_message(message)

    def _process_recv_message(
        self, message: tuple[int, Board | Game, EdaxEvaluations]
    ) -> None:
        level, board_or_game, evaluations = message
        self.all_evaluations.update(evaluations)

        next_level = level + 2
        if next_level <= 24:
            if isinstance(board_or_game, Game):
                start_game_evaluation(next_level, board_or_game, self.recv_queue)

            elif self.get_board() == board_or_game:
                start_board_evaluation(next_level, board_or_game, self.recv_queue)

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
