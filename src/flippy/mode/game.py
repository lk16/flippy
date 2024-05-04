import pygame
from pygame.event import Event

from flippy.arguments import Arguments
from flippy.mode.base import BaseMode
from flippy.othello.board import Board
from flippy.othello.position import InvalidMove


class GameMode(BaseMode):
    def __init__(self, args: Arguments) -> None:
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

        try:
            child = self.get_board().do_move(move)
        except InvalidMove:
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
