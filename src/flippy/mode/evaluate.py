import queue
import requests
from multiprocessing import Queue
from pygame.event import Event
from typing import Any

from flippy.arguments import Arguments
from flippy.book import MAX_UI_SEARCH_LEVEL, MIN_UI_SEARCH_LEVEL, is_savable_evaluation
from flippy.book.models import SerializedEvaluation
from flippy.config import get_book_server_token, get_book_server_url
from flippy.edax.process import start_evaluation
from flippy.edax.types import EdaxEvaluations, EdaxRequest, EdaxResponse
from flippy.mode.game import GameMode
from flippy.othello.position import Position


class EvaluateMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)
        self.recv_queue: Queue[EdaxResponse] = Queue()
        self.evaluations = EdaxEvaluations()
        self.server_url = get_book_server_url()
        self.token = get_book_server_token()

        self.on_board_change()

    def on_move(self, move: int) -> None:
        board = self.get_board()
        super().on_move(move)
        if self.get_board() != board:
            self.on_board_change()

    def on_mouse_right_click(self, event: Event) -> None:
        board = self.get_board()
        super().on_mouse_right_click(event)
        if self.get_board() != board:
            self.on_board_change()

    def on_board_change(self) -> None:
        position = self.get_board().position

        # Get positions that are missing and not currently being evaluated
        missing_positions = self.evaluations.get_missing_children(position)
        if not missing_positions:
            return

        # Fetch evaluations from server API only for positions not being evaluated
        response = requests.get(
            f"{self.server_url}/api/positions",
            json=[pos.to_api() for pos in missing_positions],
            headers={"x-token": self.token},
        )
        response.raise_for_status()

        server_evaluations = [
            SerializedEvaluation.model_validate(item) for item in response.json()
        ]
        evaluations = {
            Position.from_api(e.position): e.to_evaluation() for e in server_evaluations
        }
        self.evaluations.update(evaluations)

        # Check again for any positions still missing after server update
        missing_positions = self.evaluations.get_missing_children(position)

        request = EdaxRequest(missing_positions, MIN_UI_SEARCH_LEVEL, source=position)
        start_evaluation(request, self.recv_queue)

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

        positions = message.request.positions
        level = message.request.level
        source = message.request.source
        next_level = level + 2

        if (
            isinstance(source, Position)
            and source == self.get_board().position
            and next_level <= MAX_UI_SEARCH_LEVEL
        ):
            next_request = EdaxRequest(positions, next_level, source=source)
            start_evaluation(next_request, self.recv_queue)

    def get_ui_details(self) -> dict[str, Any]:
        self._process_recv_messages()

        evaluations: dict[int, dict[str, int]] = {}

        board = self.get_board()

        for move in board.get_moves_as_set():
            child = board.do_move(move)

            try:
                evaluation = self.evaluations.lookup(child.position)
            except KeyError:
                continue

            evaluations[move] = {
                "score": -evaluation.score,
                "level": evaluation.level,
            }

        return {"evaluations": evaluations}
