import os
from pathlib import Path
from flippy.openings_training import OpeningsTraining
from flippy.watch import BoardNotFound, FlyOrDieWatcher
from flippy.board import BLACK, UNKNOWN, WHITE, WRONG_MOVE, Board, ROWS, COLS

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

import pygame  # noqa:E402
from pygame.event import Event  # noqa:E402

WIDTH = 600
HEIGHT = 600

SQUARE_SIZE = WIDTH // COLS

COLOR_WHITE_DISC = (255, 255, 255)
COLOR_BLACK_DISC = (0, 0, 0)
COLOR_BACKGROUND = (0, 128, 0)
COLOR_UNKNOWN = (180, 180, 180)
COLOR_WRONG_MOVE = (255, 0, 0)

FRAME_RATE = 60

MODE_GAME = 0
MODE_WATCH = 1
MODE_OPENINGS_TRAINING = 2


class Window:
    def __init__(self) -> None:
        pygame.init()
        self.mode = MODE_OPENINGS_TRAINING  # TODO #7 use UI / env var to toggle
        self.history = [Board.start()]
        self.screen = pygame.display.set_mode((WIDTH, HEIGHT))
        self.running = False
        self.clock = pygame.time.Clock()
        self.watcher = FlyOrDieWatcher()
        self.openings_training = OpeningsTraining(
            Path(__file__).parent / "../../openings.json"
        )
        pygame.display.set_caption("Flippy")

    def get_board(self) -> Board:
        return self.history[-1]

    def run(self) -> None:
        self.running = True

        while self.running:
            for event in pygame.event.get():
                self.on_event(event)

            if self.mode == MODE_WATCH:
                self.load_board_from_screen()

            if self.mode == MODE_OPENINGS_TRAINING:
                self.history = [self.openings_training.get_board()]

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

            if board.squares[offset] == WHITE:
                pygame.draw.circle(
                    self.screen, COLOR_WHITE_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )
            elif board.squares[offset] == BLACK:
                pygame.draw.circle(
                    self.screen, COLOR_BLACK_DISC, square_centre, SQUARE_SIZE // 2 - 5
                )
            elif board.squares[offset] == UNKNOWN:
                pygame.draw.rect(
                    self.screen,
                    COLOR_UNKNOWN,
                    (col * SQUARE_SIZE, row * SQUARE_SIZE, SQUARE_SIZE, SQUARE_SIZE),
                )
            elif board.squares[offset] == WRONG_MOVE:
                pygame.draw.circle(
                    self.screen, COLOR_WRONG_MOVE, square_centre, SQUARE_SIZE // 8
                )

            if board.is_valid_move(offset):
                if board.turn == WHITE:
                    pygame.draw.circle(
                        self.screen, COLOR_WHITE_DISC, square_centre, SQUARE_SIZE // 8
                    )
                elif board.turn == BLACK:
                    pygame.draw.circle(
                        self.screen, COLOR_BLACK_DISC, square_centre, SQUARE_SIZE // 8
                    )

        pygame.display.flip()

    def on_event(self, event: Event) -> None:
        if event.type == pygame.QUIT:
            self.running = False

        if self.mode == MODE_WATCH:
            return

        if event.type == pygame.MOUSEBUTTONDOWN:
            if event.button == 1:
                self.on_mouse_left_click(event)
            if event.button == 3:
                if self.mode == MODE_OPENINGS_TRAINING:
                    return
                self.on_mouse_right_click(event)

    def on_mouse_left_click(self, event: Event) -> None:
        if self.mode == MODE_GAME and self.get_board().is_game_end():
            # Restart game
            self.history = [Board.start()]
            return

        mouseX, mouseY = event.pos
        clicked_col = mouseX // SQUARE_SIZE
        clicked_row = mouseY // SQUARE_SIZE

        if not (0 <= clicked_row < ROWS and 0 <= clicked_col < COLS):
            return

        move = clicked_row * COLS + clicked_col

        if self.mode == MODE_OPENINGS_TRAINING:
            self.openings_training.on_click(move)
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

    def load_board_from_screen(self) -> None:
        try:
            board = self.watcher.get_board()
        except BoardNotFound:
            return

        self.history = [board]


def main() -> None:
    Window().run()
