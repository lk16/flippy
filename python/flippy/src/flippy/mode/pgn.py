import pygame
import queue
import threading
import tkinter as tk
from multiprocessing import Queue
from pathlib import Path
from pygame.event import Event
from tkinter import filedialog
from typing import Any, Optional

from flippy.arguments import Arguments
from flippy.book import MAX_UI_SEARCH_LEVEL, MIN_UI_SEARCH_LEVEL
from flippy.book.api_client import APIClient
from flippy.edax.process import start_evaluation
from flippy.edax.types import EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.mode.base import BaseMode
from flippy.othello.board import BLACK, Board
from flippy.othello.game import Game
from flippy.othello.position import InvalidMove, NormalizedPosition, Position


class PGNMode(BaseMode):
    def __init__(self, args: Arguments) -> None:
        self.args = args.pgn
        self.game: Optional[Game] = None
        self.game_board_index = 0
        self.alternative_moves: list[Board] = []
        self.recv_queue: Queue[EdaxResponse | EdaxEvaluations] = Queue()
        self.evaluations = EdaxEvaluations()
        self.show_all_move_evaluations = False
        self.show_level = False
        self.api_client = APIClient()

        if self.args.pgn_file:
            self.game = Game.from_pgn(self.args.pgn_file)
            self.search_game_positions(self.game, MIN_UI_SEARCH_LEVEL)

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
            elif event.key == pygame.K_l:
                self.toggle_show_level()

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
            next_board = self.game.boards[self.game_board_index + 1]
        except IndexError:
            next_board = None

        if next_board is not None and next_board == child:
            # User clicked on square that was actually played in game.
            # We do not handle it as alternative move.
            self.game_board_index += 1
        else:
            self.alternative_moves.append(child)

        if not child.has_moves() and child.pass_move().has_moves():
            # Opponent ran out of moves, but the game is not over.
            child = child.pass_move()

            if self.alternative_moves:
                self.alternative_moves.pop()
                self.alternative_moves.append(child)

        self.search_child_positions(child.position, MIN_UI_SEARCH_LEVEL)

    def toggle_show_all_move_evaluations(self) -> None:
        self.show_all_move_evaluations = not self.show_all_move_evaluations

    def toggle_show_level(self) -> None:
        self.show_level = not self.show_level

    def show_next_position(self) -> None:
        if self.game is None:
            return

        if self.alternative_moves:
            return

        if self.game_board_index < len(self.game.boards) - 1:
            self.game_board_index += 1

    def show_prev_position(self) -> None:
        if self.game is None:
            return

        if self.alternative_moves:
            self.alternative_moves.pop()
            return

        if self.game_board_index > 0:
            self.game_board_index -= 1

    def get_board(self) -> Board:
        if self.game is None:
            return Board.start()

        if self.alternative_moves:
            return self.alternative_moves[-1]

        return self.game.boards[self.game_board_index]

    def get_played_move(self) -> Optional[int]:
        if self.game is None:
            # No game selected
            return None

        if self.alternative_moves:
            # User played some moves that are not part of the game.
            return None

        current_board = self.game.boards[self.game_board_index]

        try:
            next_board = self.game.boards[self.game_board_index + 1]
        except IndexError:
            # Game ended with current board.
            return None

        for move in current_board.get_moves_as_set():
            if current_board.do_move(move) == next_board:
                return move

        # Last move was a pass move.
        assert not current_board.has_moves()
        return None

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
        self.game_board_index = 0

        self.search_game_positions(self.game, MIN_UI_SEARCH_LEVEL)
        return pgn_file

    def _process_recv_messages(self) -> None:
        while True:
            try:
                message = self.recv_queue.get_nowait()
            except queue.Empty:
                break

            if isinstance(message, EdaxResponse):
                self._process_recv_edax_response(message)
            elif isinstance(message, EdaxEvaluations):
                self._process_recv_edax_evaluations(message)

    def _process_recv_edax_evaluations(self, evaluations: EdaxEvaluations) -> None:
        self.evaluations.update(evaluations)

    def _process_recv_edax_response(self, response: EdaxResponse) -> None:
        self.evaluations.update(response.evaluations)

        # Submit evaluations to server API
        payload = [
            eval for eval in response.evaluations.values() if eval.is_db_savable()
        ]

        def save_evaluations() -> None:
            self.api_client.save_learned_evaluations(payload)

        # Post to API in separate thread to avoid blocking the main thread.
        threading.Thread(target=save_evaluations).start()

        next_search_level = response.request.level + 2

        if next_search_level > MAX_UI_SEARCH_LEVEL:
            return

        source = response.request.source

        if isinstance(source, Game):
            if source != self.game:
                # Game changed, don't evaluate further
                return

            self.search_game_positions(source, next_search_level)

        elif isinstance(source, Position):
            if source != self.get_board().position:
                # Board changed, don't evaluate further
                return

            self.search_child_positions(source, next_search_level)

    def get_ui_evaluations(self) -> dict[int, dict[str, int]]:
        evaluations: dict[int, dict[str, int]] = {}
        board = self.get_board()
        played_move = self.get_played_move()

        for move in board.get_moves_as_set():
            child = board.do_move(move)

            try:
                evaluation = self.evaluations[child.position.normalized()]
            except KeyError:
                continue

            evaluations[move] = {"score": -evaluation.score}

            if self.show_level:
                evaluations[move]["level"] = evaluation.level

        if (
            not self.alternative_moves
            and evaluations
            and not self.show_all_move_evaluations
        ):
            max_evaluation = max(evaluations.values(), key=lambda x: x["score"])

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
                    evaluation = self.evaluations[child.position.normalized()]
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
            graph_current_move = self.game_board_index

        ui_details["graph_current_move"] = graph_current_move
        return ui_details

    def get_positions_from_api_threaded(
        self, positions: set[NormalizedPosition]
    ) -> None:
        def call_api() -> None:
            found_evaluations = self.api_client.lookup_positions(positions)
            self.recv_queue.put(found_evaluations)

        if positions:
            threading.Thread(target=call_api).start()

    def search_game_positions(self, game: Game, level: int) -> None:
        positions = game.get_normalized_positions(add_children=True)

        missing_positions = self.evaluations.get_missing(positions)
        self.get_positions_from_api_threaded(missing_positions)

        self._search_missing_positions(positions, level, game)

    def search_child_positions(self, parent: Position, level: int) -> None:
        positions = parent.get_normalized_children()

        missing_positions = self.evaluations.get_missing(positions)
        self.get_positions_from_api_threaded(missing_positions)

        self._search_missing_positions(positions, level, parent)

    def _search_missing_positions(
        self, positions: set[NormalizedPosition], level: int, source: Game | Position
    ) -> None:
        if level > MAX_UI_SEARCH_LEVEL:
            return

        learn_positions = set()
        for position in positions:
            if position not in self.evaluations:
                learn_positions.add(position)
                continue

            evaluation = self.evaluations[position]

            if evaluation.level < level:
                learn_positions.add(position)

        if not learn_positions:
            self._search_missing_positions(positions, level + 2, source)
            return

        request = EdaxRequest(learn_positions, level, source=source)

        # Ignoring type error is fine here since start_evaluation expects Queue[EdaxResponse],
        # but we have Queue[EdaxResponse | EdaxEvaluations]. EdaxResponse is a subset of our queue's types.
        start_evaluation(request, self.recv_queue)  # type:ignore[arg-type]
