from PIL.Image import Image
from math import sqrt
from typing import Optional

from flippy.board import EMPTY, BLACK, WHITE, UNKNOWN, Board, opponent

import pyautogui

FOD_LEFT_TOP_MARKER = (53, 144, 103)
FOD_RIGHT_TOP_MARKER = (64, 134, 169)
FOD_WHITE = (223, 225, 227)
FOD_BLACK = (48, 48, 48)
FOD_EMPTY = (7, 96, 0)
FOD_TURN_HIGHLIGHTER = (182, 170, 65)


def is_similar_color(
    lhs: tuple[int, int, int], rhs: tuple[int, int, int], max_distance: float
) -> bool:
    d_red = lhs[0] - rhs[0]
    d_green = lhs[1] - rhs[1]
    d_blue = lhs[2] - rhs[2]

    distance = sqrt(d_red**2 + d_green**2 + d_blue**2)
    return distance < max_distance


class BoardNotFound(Exception):
    ...


class FlyOrDieWatchCoords:
    def __init__(self, left_x: int, right_x: int, y: int) -> None:
        self.left_x = left_x
        self.right_x = right_x
        self.y = y

    def scale_factor(self) -> int:
        return self.right_x - self.left_x

    def get_square_centre_coords(self, offset: int) -> tuple[int, int]:
        a1_x_centre = self.left_x + self.scale_factor() * 0.053
        a1_y_centre = self.y + self.scale_factor() * 0.053
        field_size_px = self.scale_factor() * 0.072

        x = int(a1_x_centre + (offset % 8) * field_size_px)
        y = int(a1_y_centre + (offset // 8) * field_size_px)
        return x, y

    def get_turn_highlighter_coords(self) -> tuple[int, int]:
        x = self.left_x + int(self.scale_factor() * 0.649)
        y = self.y + int(self.scale_factor() * 0.103)
        return x, y

    def get_turn_color_coords(self) -> tuple[int, int]:
        x = self.left_x + int(self.scale_factor() * 0.649)
        y = self.y + int(self.scale_factor() * 0.125)
        return x, y

    def is_valid_coords(self, screenshot: Image) -> bool:
        left_top_pixel = screenshot.getpixel((self.left_x, self.y))
        right_top_pixel = screenshot.getpixel((self.right_x, self.y))

        return (
            is_similar_color(left_top_pixel, FOD_LEFT_TOP_MARKER, 20)
            and is_similar_color(right_top_pixel, FOD_RIGHT_TOP_MARKER, 20)
            and self.right_x - self.left_x > 400
        )


class FlyOrDieWatcher:
    def __init__(self) -> None:
        self.prev_coords: Optional[FlyOrDieWatchCoords] = None
        self.screenshot: Image

    def get_board(self) -> Board:
        self.screenshot = pyautogui.screenshot()

        if self.prev_coords:
            try:
                return self.get_board_from_coords(self.prev_coords)
            except BoardNotFound:
                if self.prev_coords.is_valid_coords(self.screenshot):
                    # No board is visible, but anchor coords are still the same.
                    # This prevents a full re-search of the screenshot for anchors.
                    raise BoardNotFound

        width, height = self.screenshot.size

        for y in range(height):
            left_x: Optional[int] = None
            right_x: Optional[int] = None
            for x in range(width):
                pixel = self.screenshot.getpixel((x, y))

                if is_similar_color(pixel, FOD_LEFT_TOP_MARKER, 20) and left_x is None:
                    left_x = x

                if is_similar_color(pixel, FOD_RIGHT_TOP_MARKER, 20):
                    right_x = x

            if left_x is not None and right_x is not None:
                coords = FlyOrDieWatchCoords(left_x, right_x, y)

                if coords.is_valid_coords(self.screenshot):
                    board = self.get_board_from_coords(coords)
                    self.prev_coords = coords
                    return board

        raise BoardNotFound

    def get_board_from_coords(self, coords: FlyOrDieWatchCoords) -> Board:
        squares = [EMPTY] * 64

        for i in range(64):
            centre = coords.get_square_centre_coords(i)
            squares[i] = self.get_square_at_coords(centre)

        turn = self.get_turn(coords)

        board = Board(squares, turn)

        if board.count(UNKNOWN) > 0:
            raise BoardNotFound

        return board

    def get_square_at_coords(self, coord: tuple[int, int]) -> int:
        pixel = self.screenshot.getpixel(coord)

        if is_similar_color(pixel, FOD_BLACK, 50):
            return BLACK
        elif is_similar_color(pixel, FOD_WHITE, 50):
            return WHITE
        elif is_similar_color(pixel, FOD_EMPTY, 50):
            return EMPTY
        return UNKNOWN

    def get_turn(self, coords: FlyOrDieWatchCoords) -> int:
        turn_highlighter_coords = coords.get_turn_highlighter_coords()
        turn_highlighter = self.screenshot.getpixel(turn_highlighter_coords)

        turn_color_coords = coords.get_turn_color_coords()
        turn = self.get_square_at_coords(turn_color_coords)

        if turn not in [WHITE, BLACK]:
            raise BoardNotFound

        if is_similar_color(turn_highlighter, FOD_TURN_HIGHLIGHTER, 50):
            return turn
        return opponent(turn)
