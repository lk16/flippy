import os

from src.flippy.board import Board, ROWS, COLS

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

import pygame  # noqa:E402
from pygame.event import Event  # noqa:E402

WIDTH = 400
HEIGHT = 400

SQUARE_SIZE = WIDTH // COLS

COLOR_WHITE_DISC = (255, 255, 255)
COLOR_BLACK_DISC = (0, 0, 0)
COLOR_BACKGROUND = (0, 128, 0)

FRAME_RATE = 60


class Window:
    def __init__(self) -> None:
        pygame.init()
        self.history = [Board.start()]
        self.screen = pygame.display.set_mode((WIDTH, HEIGHT))
        self.running = False
        self.clock = pygame.time.Clock()
        pygame.display.set_caption("Flippy")

    def get_board(self) -> Board:
        return self.history[-1]

    def run(self) -> None:
        self.running = True

        while self.running:
            for event in pygame.event.get():
                self.on_event(event)

            self.draw()

            self.clock.tick(FRAME_RATE)

        pygame.quit()

    def draw(self) -> None:
        board = self.get_board()

        self.screen.fill(COLOR_BACKGROUND)
        for offset in range(ROWS * COLS):
            col = offset % COLS
            row = offset // COLS

            pygame.draw.rect(
                self.screen,
                COLOR_BACKGROUND,
                (col * SQUARE_SIZE, row * SQUARE_SIZE, SQUARE_SIZE, SQUARE_SIZE),
            )

            square_centre = (
                col * SQUARE_SIZE + SQUARE_SIZE // 2,
                row * SQUARE_SIZE + SQUARE_SIZE // 2,
            )

            if board.squares[offset] == 1:
                pygame.draw.circle(
                    self.screen, COLOR_WHITE_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )
            elif board.squares[offset] == -1:
                pygame.draw.circle(
                    self.screen, COLOR_BLACK_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )

            if board.is_valid_move(offset):
                if board.turn == 1:
                    pygame.draw.circle(
                        self.screen, COLOR_WHITE_DISC, square_centre, SQUARE_SIZE // 8
                    )
                elif board.turn == -1:
                    pygame.draw.circle(
                        self.screen, COLOR_BLACK_DISC, square_centre, SQUARE_SIZE // 8
                    )

        pygame.display.flip()

    def on_event(self, event: Event) -> None:
        if event.type == pygame.QUIT:
            self.running = False
        elif event.type == pygame.MOUSEBUTTONDOWN:
            if event.button == 1:
                self.on_mouse_left_click(event)
            if event.button == 3:
                self.on_mouse_right_click(event)

    def on_mouse_left_click(self, event: Event) -> None:
        if self.get_board().is_game_end():
            # Restart game
            self.history = [Board.start()]
            return

        mouseX, mouseY = event.pos
        clicked_col = mouseX // SQUARE_SIZE
        clicked_row = mouseY // SQUARE_SIZE

        if not (0 <= clicked_row < ROWS and 0 <= clicked_col < COLS):
            return

        move = clicked_row * COLS + clicked_col
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
