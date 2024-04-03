from flippy.arguments import Arguments
from flippy.mode.pgn import PGNMode
from flippy.othello.board import BLACK, WHITE


import pygame
from pygame.event import Event

WIDTH = 600
HEIGHT = 600

SQUARE_SIZE = WIDTH // 8
DISC_RADIUS = SQUARE_SIZE // 2 - 5
MOVE_INDICATOR_RADIUS = SQUARE_SIZE // 8

FONT_SIZE = 50

COLOR_WHITE_DISC = (255, 255, 255)
COLOR_BLACK_DISC = (0, 0, 0)
COLOR_GRAY_DISC = (128, 128, 128)
COLOR_BACKGROUND = (0, 128, 0)
COLOR_UNKNOWN = (180, 180, 180)
COLOR_WRONG_MOVE = (255, 0, 0)

FRAME_RATE = 60


class NonMoveEvent(Exception):
    pass


class Window:
    def __init__(self, args: Arguments) -> None:
        pygame.init()
        self.args = args

        # TODO #7 use UI / env var to toggle
        self.mode = PGNMode(args)

        self.screen = pygame.display.set_mode((WIDTH, HEIGHT))
        self.clock = pygame.time.Clock()

        pygame.display.set_caption("Flippy")

    def run(self) -> None:
        running = True

        while running:
            for event in pygame.event.get():
                if event.type == pygame.QUIT:
                    running = False
                    break

                try:
                    move = self.get_move_from_event(event)
                except NonMoveEvent:
                    self.mode.on_event(event)
                else:
                    self.mode.on_move(move)

            self.mode.on_frame(event)
            self.draw()
            self.clock.tick(FRAME_RATE)

        pygame.quit()

    def get_board_square_center(self, index: int) -> tuple[int, int]:
        col = index % 8
        row = index // 8

        x = col * SQUARE_SIZE + SQUARE_SIZE // 2
        y = row * SQUARE_SIZE + SQUARE_SIZE // 2

        return (x, y)

    def draw_disc(self, index: int, color: tuple[int, int, int]) -> None:
        center = self.get_board_square_center(index)
        pygame.draw.circle(self.screen, color, center, DISC_RADIUS)

    def draw_move_indicator(self, index: int, color: tuple[int, int, int]) -> None:
        center = self.get_board_square_center(index)
        pygame.draw.circle(self.screen, color, center, MOVE_INDICATOR_RADIUS)

    def draw_best_move_marker(self, index: int, color: tuple[int, int, int]) -> None:
        center = self.get_board_square_center(index)
        radius = (SQUARE_SIZE / 2) - 8
        pygame.draw.circle(self.screen, color, center, radius, 1)

    def draw_number(self, index: int, color: tuple[int, int, int], number: int) -> None:
        if number < 100:
            font_size = FONT_SIZE
        elif number < 1000:
            font_size = int(0.75 * FONT_SIZE)
        else:
            font_size = int(0.5 * FONT_SIZE)

        font = pygame.font.Font(None, font_size)
        text_surface = font.render(str(number), True, color)
        text_rect = text_surface.get_rect()
        text_rect.center = self.get_board_square_center(index)
        self.screen.blit(text_surface, text_rect.topleft)

    def draw(self) -> None:
        board = self.mode.get_board()

        ui_details = self.mode.get_ui_details()
        move_mistakes: set[int] = ui_details.pop("move_mistakes", set())
        unknown_squares: set[int] = ui_details.pop("unknown_squares", set())
        child_frequencies: dict[int, int] = ui_details.pop("child_frequencies", {})
        evaluations: dict[int, int] = ui_details.pop("evaluations", {})

        if ui_details:
            print(
                "WARNING: found unused ui details key(s): "
                + ", ".join(sorted(ui_details))
            )

        if board.turn == WHITE:
            turn_color = COLOR_WHITE_DISC
        else:
            turn_color = COLOR_BLACK_DISC

        self.screen.fill(COLOR_BACKGROUND)

        for index in range(64):
            square = board.get_square(index)

            if square == WHITE:
                self.draw_disc(index, COLOR_WHITE_DISC)
            elif square == BLACK:
                self.draw_disc(index, COLOR_BLACK_DISC)
            elif index in unknown_squares:
                self.draw_disc(index, COLOR_GRAY_DISC)
            elif index in move_mistakes:
                self.draw_move_indicator(index, COLOR_WRONG_MOVE)
            elif index in child_frequencies:
                self.draw_number(index, turn_color, child_frequencies[index])
            elif index in evaluations:
                self.draw_number(index, turn_color, evaluations[index])
                if evaluations[index] == max(evaluations.values()):
                    self.draw_best_move_marker(index, turn_color)
            elif board.is_valid_move(index):
                self.draw_move_indicator(index, turn_color)

        pygame.display.flip()

    def get_move_from_event(self, event: Event) -> int:
        if event.type != pygame.MOUSEBUTTONDOWN:
            raise NonMoveEvent

        if event.button != pygame.BUTTON_LEFT:
            raise NonMoveEvent

        x, y = event.pos
        col: int = x // SQUARE_SIZE
        row: int = y // SQUARE_SIZE

        if not (row in range(8) and col in range(8)):
            raise NonMoveEvent

        return row * 8 + col
