import os

from src.flippy.board import Board, ROWS, COLS

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

import pygame  # noqa:E402
from pygame.event import Event  # noqa:E402

# Constants
WIDTH, HEIGHT = 400, 400

SQUARE_SIZE = WIDTH // COLS

# Colors
WHITE = (255, 255, 255)
BLACK = (0, 0, 0)
GREEN = (0, 128, 0)


class Window:
    def __init__(self) -> None:
        pygame.init()
        self.board = Board()
        self.screen = pygame.display.set_mode((WIDTH, HEIGHT))
        self.running = False
        pygame.display.set_caption("Flippy")

    def run(self) -> None:
        self.running = True

        while self.running:
            for event in pygame.event.get():
                self.on_event(event)

            self.draw()

            # Check for game over
            if self.board.is_game_end():
                self.running = False

        pygame.quit()

    def draw(self) -> None:
        self.screen.fill(GREEN)
        for row in range(ROWS):
            for col in range(COLS):
                pygame.draw.rect(
                    self.screen,
                    GREEN,
                    (col * SQUARE_SIZE, row * SQUARE_SIZE, SQUARE_SIZE, SQUARE_SIZE),
                )
                if self.board.squares[row][col] == 1:
                    pygame.draw.circle(
                        self.screen,
                        WHITE,
                        (
                            col * SQUARE_SIZE + SQUARE_SIZE // 2,
                            row * SQUARE_SIZE + SQUARE_SIZE // 2,
                        ),
                        SQUARE_SIZE // 2 - 5,
                    )
                elif self.board.squares[row][col] == -1:
                    pygame.draw.circle(
                        self.screen,
                        BLACK,
                        (
                            col * SQUARE_SIZE + SQUARE_SIZE // 2,
                            row * SQUARE_SIZE + SQUARE_SIZE // 2,
                        ),
                        SQUARE_SIZE // 2 - 5,
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
                self.board.do_move(clicked_row, clicked_col)
