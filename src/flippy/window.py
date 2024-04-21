import pygame
from math import floor
from pygame.event import Event
from typing import Optional

from flippy.arguments import Arguments
from flippy.mode.evaluate import EvaluateMode
from flippy.othello.board import BLACK, WHITE

BOARD_WIDTH_PX = 600
BOARD_HEIGHT_PX = 600
GRAPH_HEIGHT_PX = 200

SQUARE_SIZE = BOARD_WIDTH_PX // 8
DISC_RADIUS = SQUARE_SIZE // 2 - 5
MOVE_INDICATOR_RADIUS = SQUARE_SIZE // 8

FONT_SIZE = 50

COLOR_WHITE_DISC = (255, 255, 255)
COLOR_BLACK_DISC = (0, 0, 0)
COLOR_GRAY_DISC = (128, 128, 128)
COLOR_BACKGROUND = (0, 128, 0)
COLOR_UNKNOWN = (180, 180, 180)
COLOR_WRONG_MOVE = (255, 0, 0)
COLOR_PLAYED_MOVE = (0, 96, 0)
COLOR_SCORE_LINE = (96, 96, 96)


FRAME_RATE = 60


class NonMoveEvent(Exception):
    pass


class Window:
    def __init__(self, args: Arguments) -> None:
        pygame.init()
        self.args = args

        # TODO #7 use UI / env var to toggle
        self.mode = EvaluateMode(args)

        self.screen = pygame.display.set_mode((BOARD_WIDTH_PX, BOARD_HEIGHT_PX))
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
        played_move: Optional[int] = ui_details.pop("played_move", None)
        graph_data: list[Optional[tuple[int, int]]] = ui_details.pop("graph_data", [])
        graph_current_move: Optional[int] = ui_details.pop("graph_current_move", None)

        if ui_details:
            print(
                "WARNING: found unused ui details key(s): "
                + ", ".join(sorted(ui_details))
            )

        if graph_data:
            height = BOARD_HEIGHT_PX + GRAPH_HEIGHT_PX
        else:
            height = BOARD_HEIGHT_PX

        if self.screen.get_height() != height:
            self.screen = pygame.display.set_mode((BOARD_WIDTH_PX, height))

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

            if played_move == index:
                self.draw_disc(index, COLOR_PLAYED_MOVE)

            if index in evaluations:
                self.draw_number(index, turn_color, evaluations[index])
                if evaluations[index] == max(evaluations.values()):
                    self.draw_best_move_marker(index, turn_color)
            elif board.is_valid_move(index) and not child_frequencies:
                self.draw_move_indicator(index, turn_color)

        self.draw_graph(graph_data, graph_current_move)

        pygame.display.flip()

    def draw_graph(
        self,
        graph_data: list[Optional[tuple[int, int]]],
        graph_current_move: Optional[int],
    ) -> None:
        if not graph_data:
            return

        big_margin = 40
        small_margin = 10

        x_min = big_margin
        y_min = BOARD_HEIGHT_PX + small_margin
        x_max = BOARD_WIDTH_PX - small_margin
        y_max = BOARD_HEIGHT_PX + GRAPH_HEIGHT_PX - small_margin

        pygame.draw.rect(
            self.screen,
            COLOR_GRAY_DISC,
            ((x_min, y_min), (x_max - x_min, y_max - y_min)),
        )

        # Graph considers scores from black's POV: positive values means black has an advantage.
        # Reason: this is best practice, likely choice for black is because black plays first.
        valid_black_scores = [item[1] for item in graph_data if item is not None]

        if not valid_black_scores:
            return

        min_black_score = min(valid_black_scores + [-4])
        max_black_score = max(valid_black_scores + [4])
        score_range = max_black_score - min_black_score

        if score_range <= 20:
            y_line_interval = 4
        elif score_range <= 40:
            y_line_interval = 8
        elif score_range <= 60:
            y_line_interval = 16
        else:
            y_line_interval = 32

        # Draw lines
        for black_score in range(-64, 65, y_line_interval):
            if not (min_black_score <= black_score <= max_black_score):
                continue

            y = y_min + (y_max - y_min) * (max_black_score - black_score) / score_range
            pygame.draw.line(self.screen, COLOR_SCORE_LINE, (x_min, y), (x_max, y))

            font = pygame.font.Font(None, 25)

            if black_score == 0:
                text = "0"
                text_color = COLOR_GRAY_DISC
            else:
                text = "+" + str(abs(black_score))
                if black_score < 0:
                    text_color = COLOR_WHITE_DISC
                else:
                    text_color = COLOR_BLACK_DISC

            text_surface = font.render(text, True, text_color)
            text_rect = text_surface.get_rect()
            text_rect.center = (x_min // 2, floor(y))
            self.screen.blit(text_surface, text_rect.topleft)

        circles: list[tuple[tuple[int, int, int], tuple[int, int], int]] = []
        prev_dot: Optional[tuple[int, int]] = None

        for offset, graph_data_item in enumerate(graph_data):
            if graph_data_item is None:
                continue

            turn, black_score = graph_data_item

            x = x_min + (x_max - x_min) * (offset / (len(graph_data) - 1))
            y = y_min + (y_max - y_min) * (
                (max_black_score - black_score) / score_range
            )
            dot = (floor(x), floor(y))

            if offset == graph_current_move:
                circle_size = 5
            else:
                circle_size = 3

            if turn == BLACK:
                dot_color = COLOR_BLACK_DISC
            else:
                dot_color = COLOR_WHITE_DISC

            if prev_dot is not None:
                pygame.draw.line(self.screen, COLOR_BLACK_DISC, prev_dot, dot)

            prev_dot = dot

            circle = dot_color, dot, circle_size
            circles.append(circle)

        # Draw circles last, so lines don't overlap with them
        for dot_color, dot, circle_size in circles:
            pygame.draw.circle(self.screen, dot_color, dot, circle_size)

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
