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
        self.board = Board.start()
        self.screen = pygame.display.set_mode((WIDTH, HEIGHT))
        self.running = False
        self.clock = pygame.time.Clock()
        pygame.display.set_caption("Flippy")

    def run(self) -> None:
        self.running = True

        while self.running:
            for event in pygame.event.get():
                self.on_event(event)

            self.draw()

            if self.board.is_game_end():
                self.running = False

            self.clock.tick(FRAME_RATE)

        pygame.quit()

    def draw(self) -> None:
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

            if self.board.squares[offset] == 1:
                pygame.draw.circle(
                    self.screen, COLOR_WHITE_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )
            elif self.board.squares[offset] == -1:
                pygame.draw.circle(
                    self.screen, COLOR_BLACK_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )

            if self.board.is_valid_move(offset):
                if self.board.turn == 1:
                    pygame.draw.circle(
                        self.screen, COLOR_WHITE_DISC, square_centre, SQUARE_SIZE // 8
                    )
                elif self.board.turn == -1:
                    pygame.draw.circle(
                        self.screen, COLOR_BLACK_DISC, square_centre, SQUARE_SIZE // 8
                    )

        pygame.display.flip()

    def on_event(self, event: Event) -> None:
        if event.type == pygame.QUIT:
            self.running = False
        elif event.type == pygame.MOUSEBUTTONDOWN:
            mouseX, mouseY = event.pos
            clicked_col = mouseX // SQUARE_SIZE
            clicked_row = mouseY // SQUARE_SIZE

            if 0 <= clicked_row < ROWS and 0 <= clicked_col < COLS:
                move = clicked_row * COLS + clicked_col
                child = self.board.do_move(move)

                if child:
                    self.board = child

                    # Pass if there are no moves
                    if not self.board.has_moves():
                        passed = self.board.pass_move()

                        if passed.has_moves():
                            self.board = passed
