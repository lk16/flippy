import pygame
import queue
import requests
import tkinter as tk
from multiprocessing import Queue
from pathlib import Path
from pygame.event import Event
from tkinter import filedialog
from typing import Any, Iterable, Optional

from flippy.arguments import Arguments
from flippy.book import MAX_UI_SEARCH_LEVEL, MIN_UI_SEARCH_LEVEL, is_savable_evaluation
from flippy.book.models import SerializedEvaluation
from flippy.config import get_book_server_token, get_book_server_url
from flippy.edax.process import start_evaluation
from flippy.edax.types import EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.mode.base import BaseMode
from flippy.othello.board import BLACK, Board
from flippy.othello.game import Game
from flippy.othello.position import InvalidMove, Position


class PGNMode(BaseMode):
    def __init__(self, args: Arguments) -> None:
        self.args = args.pgn
        self.game: Optional[Game] = None
        self.moves_done = 0
        self.alternative_moves: list[Board] = []
        self.recv_queue: Queue[EdaxResponse] = Queue()
        self.evaluations = EdaxEvaluations()
        self.show_all_move_evaluations = False
        self.server_url = get_book_server_url()
        self.token = get_book_server_token()

        if self.args.pgn_file:
            self.game = Game.from_pgn(self.args.pgn_file)
            self.lookup_or_search(
                self.game.get_normalized_positions(add_children=True), source=self.game
            )

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
            elif event.key == pygame.K_SPACE:
                self.toggle_show_all_move_evaluations()

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
            next_board = None

        if next_board is not None and next_board == child:
            # User clicked on square that was actually played in game.
            # We do not handle it as alternative move.
            self.moves_done += 1
        else:
            self.alternative_moves.append(child)

        if not child.has_moves() and child.pass_move().has_moves():
            # Opponent ran out of moves, but the game is not over.
            child = child.pass_move()

            if self.alternative_moves:
                self.alternative_moves.pop()
                self.alternative_moves.append(child)

        self.lookup_or_search(child.get_child_positions(), source=child)

    def toggle_show_all_move_evaluations(self) -> None:
        self.show_all_move_evaluations = not self.show_all_move_evaluations

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

        self.lookup_or_search(self.game.get_normalized_positions(), source=self.game)
        return pgn_file

    def _process_recv_messages(self) -> None:
        while True:
            try:
                message = self.recv_queue.get_nowait()
            except queue.Empty:
                break

            self._process_recv_message(message)

    def _process_recv_message(self, message: EdaxResponse) -> None:
        self.evaluations.update(message.evaluations.values)

        # Submit evaluations to server API
        payload = [
            SerializedEvaluation.from_evaluation(eval).model_dump()
            for eval in message.evaluations.values.values()
            if is_savable_evaluation(eval)
        ]

        if payload:
            response = requests.post(
                f"{self.server_url}/api/evaluations",
                json=payload,
                headers={"x-token": self.token},
            )
            response.raise_for_status()

        source = message.request.source

        if isinstance(source, Game):
            if source != self.game:
                # Game changed, don't evaluate further
                return

            positions = self.game.get_normalized_positions()

        elif isinstance(source, Board):
            if source != self.get_board():
                # Board changed, don't evaluate further
                return

            positions = set(source.get_child_positions())

        else:
            # Unreachable
            raise NotImplementedError

        self.lookup_or_search(positions, source=source)

    def get_ui_evaluations(self) -> dict[int, int]:
        evaluations: dict[int, int] = {}
        board = self.get_board()
        played_move = self.get_played_move()

        for move in board.get_moves_as_set():
            child = board.do_move(move)

            try:
                evaluation = self.evaluations.lookup(child.position)
            except KeyError:
                continue

            evaluations[move] = -evaluation.score

        if (
            not self.alternative_moves
            and evaluations
            and not self.show_all_move_evaluations
        ):
            max_evaluation = max(evaluations.values())

            shown_evaluations = {
                move
                for (move, evaluation) in evaluations.items()
                if evaluation == max_evaluation
            }

            if played_move is not None:
                shown_evaluations.add(played_move)

            evaluations = {
                move: evaluation
                for (move, evaluation) in evaluations.items()
                if move in shown_evaluations
            }

        return evaluations

    def get_ui_graph_data(self) -> list[tuple[int, int] | None]:
        if not self.game:
            return []

        graph_data: list[tuple[int, int] | None] = []

        for board in self.game.boards:
            if not board.has_moves():
                graph_data.append(None)
                continue

            # Child score is the score of the opponent.
            # We want to find the minimum score of the opponent.
            min_child_score: int | None = None

            for child in board.get_children():
                try:
                    evaluation = self.evaluations.lookup(child.position)
                except KeyError:
                    continue

                if min_child_score is None or evaluation.score < min_child_score:
                    min_child_score = evaluation.score

            if min_child_score is None:
                graph_data.append(None)
                continue

            if board.turn == BLACK:
                score = -min_child_score
            else:
                score = min_child_score

            graph_data.append((board.turn, score))

        return graph_data

    def get_ui_details(self) -> dict[str, Any]:
        self._process_recv_messages()

        ui_details: dict[str, Any] = {}

        played_move = self.get_played_move()
        if played_move is not None:
            ui_details["played_move"] = played_move

        ui_details["evaluations"] = self.get_ui_evaluations()
        ui_details["graph_data"] = self.get_ui_graph_data()

        if self.alternative_moves:
            graph_current_move = None
        else:
            graph_current_move = self.moves_done

        ui_details["graph_current_move"] = graph_current_move
        return ui_details

    def lookup_or_search(
        self, positions: Iterable[Position], *, source: Board | Game | None
    ) -> None:
        if not positions:
            return

        positions = set(positions)
        missing = set()
        for position in positions:
            missing.update(self.evaluations.get_missing_children(position))

        if missing:
            self._fetch_from_server(missing)
            missing = set()
            for position in positions:
                missing.update(self.evaluations.get_missing_children(position))
            if missing:
                self._start_initial_evaluation(missing, source)
                return

        self._start_deeper_evaluation(positions, source)

    def _fetch_from_server(self, positions: set[Position]) -> None:
        """Fetch evaluations from the server API and update local evaluations."""
        # Convert positions to API format
        positions_api = [pos.to_api() for pos in positions]

        # Process in batches of 100
        batch_size = 100
        all_evaluations = {}

        for i in range(0, len(positions_api), batch_size):
            batch = positions_api[i : i + batch_size]
            response = requests.get(
                f"{self.server_url}/api/positions",
                json=batch,
                headers={"x-token": self.token},
            )
            response.raise_for_status()

            server_evaluations = [
                SerializedEvaluation.model_validate(item) for item in response.json()
            ]
            batch_evaluations = {
                Position.from_api(e.position): e.to_evaluation()
                for e in server_evaluations
            }
            all_evaluations.update(batch_evaluations)

        self.evaluations.update(all_evaluations)

    def _start_initial_evaluation(
        self, positions: set[Position], source: Board | Game | None
    ) -> None:
        """Start initial evaluation for positions not found in server."""
        request = EdaxRequest(positions, MIN_UI_SEARCH_LEVEL, source=source)
        start_evaluation(request, self.recv_queue)

    def _start_deeper_evaluation(
        self, positions: set[Position], source: Board | Game | None
    ) -> None:
        """Start deeper evaluation for positions that have initial evaluations."""
        evaluations = [self.evaluations.lookup(position) for position in positions]
        min_level = min(evaluation.level for evaluation in evaluations)
        learn_level = min_level + 2 + (min_level % 2)

        if learn_level >= MAX_UI_SEARCH_LEVEL:
            return

        search_positions = [
            evaluation.position
            for evaluation in evaluations
            if evaluation.level == min_level
        ]

        request = EdaxRequest(search_positions, learn_level, source=source)
        start_evaluation(request, self.recv_queue)
