from flippy.board import Board
from flippy.mode.base import BaseMode

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass
from pygame.event import Event
import pygame


class GameMode(BaseMode):
    def __init__(self) -> None:
        self.history = [Board.start()]

    def on_event(self, event: Event) -> None:
        if event.type == pygame.MOUSEBUTTONDOWN:
            if event.button == pygame.BUTTON_RIGHT:
                self.on_mouse_right_click(event)

    def on_move(self, move: int) -> None:
        if self.get_board().is_game_end():
            # Restart game
            self.history = [Board.start()]
            return

        child = self.get_board().do_move(move)

        if not child:
            return

        if not child.has_moves():
            passed = child.pass_move()

            if passed.has_moves():
                child = passed

        self.history.append(child)

    def on_mouse_right_click(self, event: Event) -> None:
        # Undo last move.
        if len(self.history) > 1:
            self.history.pop()

    def get_board(self) -> Board:
        return self.history[-1]
