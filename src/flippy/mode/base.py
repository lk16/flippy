from typing import Any
from flippy.othello.board import Board
from pygame.event import Event


class BaseMode:
    def on_event(self, event: Event) -> None:
        pass

    def on_frame(self, event: Event) -> None:
        pass

    def on_move(self, move: int) -> None:
        pass

    def get_board(self) -> Board:
        raise NotImplementedError

    def get_ui_details(self) -> dict[str, Any]:
        return {}
