from typing import Optional
from flippy.mode.base import BaseMode
from flippy.mode.training.loader import ExerciseLoaderArgs
from flippy.mode.training.mode import TrainingMode
from flippy.othello.board import BLACK, WHITE


import pygame
from pygame.event import Event

WIDTH = 600
HEIGHT = 600

SQUARE_SIZE = WIDTH // 8

COLOR_WHITE_DISC = (255, 255, 255)
COLOR_BLACK_DISC = (0, 0, 0)
COLOR_GREY_DISC = (128, 128, 128)
COLOR_BACKGROUND = (0, 128, 0)
COLOR_UNKNOWN = (180, 180, 180)
COLOR_WRONG_MOVE = (255, 0, 0)

FRAME_RATE = 60


class Window:
    def __init__(self, loader_args: ExerciseLoaderArgs) -> None:
        pygame.init()
        self.mode: BaseMode = TrainingMode()  # TODO #7 use UI / env var to toggle
        self.screen = pygame.display.set_mode((WIDTH, HEIGHT))
        self.clock = pygame.time.Clock()
        pygame.display.set_caption("Flippy")

        if isinstance(self.mode, TrainingMode):
            self.mode.load_exercises(loader_args)

    def run(self) -> None:
        running = True

        while running:
            for event in pygame.event.get():
                if event.type == pygame.QUIT:
                    running = False
                    break

                move = self.get_move_from_event(event)
                if move is not None:
                    self.mode.on_move(move)

                self.mode.on_event(event)

            self.mode.on_frame(event)
            self.draw()
            self.clock.tick(FRAME_RATE)

        pygame.quit()

    def draw(self) -> None:
        board = self.mode.get_board()

        ui_details = self.mode.get_ui_details()
        move_mistakes: set[int] = ui_details.pop("move_mistakes", set())
        unknown_squares: set[int] = ui_details.pop("unknown_squares", set())

        if ui_details:
            print(
                "WARNING: found unused ui details key(s): "
                + ", ".join(sorted(ui_details))
            )

        self.screen.fill(COLOR_BACKGROUND)
        for offset in range(64):
            col = offset % 8
            row = offset // 8

            pygame.draw.rect(
                self.screen,
                COLOR_BACKGROUND,
                (col * SQUARE_SIZE, row * SQUARE_SIZE, SQUARE_SIZE, SQUARE_SIZE),
            )

            square_centre = (
                col * SQUARE_SIZE + SQUARE_SIZE // 2,
                row * SQUARE_SIZE + SQUARE_SIZE // 2,
            )

            square = board.get_square(offset)

            if square == WHITE:
                pygame.draw.circle(
                    self.screen, COLOR_WHITE_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )
            elif square == BLACK:
                pygame.draw.circle(
                    self.screen, COLOR_BLACK_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )
            elif offset in unknown_squares:
                pygame.draw.circle(
                    self.screen, COLOR_GREY_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )
            elif offset in move_mistakes:
                pygame.draw.circle(
                    self.screen, COLOR_WRONG_MOVE, square_centre, SQUARE_SIZE // 8
                )
            elif board.is_valid_move(offset):
                if board.turn == WHITE:
                    pygame.draw.circle(
                        self.screen, COLOR_WHITE_DISC, square_centre, SQUARE_SIZE // 8
                    )
                elif board.turn == BLACK:
                    pygame.draw.circle(
                        self.screen, COLOR_BLACK_DISC, square_centre, SQUARE_SIZE // 8
                    )

        pygame.display.flip()

    def get_move_from_event(self, event: Event) -> Optional[int]:
        if not (
            event.type == pygame.MOUSEBUTTONDOWN and event.button == pygame.BUTTON_LEFT
        ):
            return None

        x, y = event.pos
        col: int = x // SQUARE_SIZE
        row: int = y // SQUARE_SIZE

        if not (row in range(8) and col in range(8)):
            return None

        return row * 8 + col
