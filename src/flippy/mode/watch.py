import pyautogui
from inspect import Arguments
from math import sqrt
from PIL.Image import Image
from typing import Any, Optional

from flippy.mode.base import BaseMode
from flippy.othello.board import BLACK, EMPTY, WHITE, Board, opponent

FOD_LEFT_TOP_MARKER = (53, 144, 103)
FOD_RIGHT_TOP_MARKER = (64, 134, 169)
FOD_WHITE = (223, 225, 227)
FOD_BLACK = (48, 48, 48)
FOD_EMPTIES = [(41, 91, 25), (51, 111, 31), (0, 92, 0), (6, 113, 0)]
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
    pass


class UnknownSquare(Exception):
    pass


class FlyOrDieWatchCoords:
    def __init__(self, left_x: int, right_x: int, y: int) -> None:
        self.left_x = left_x
        self.right_x = right_x
        self.y = y

    def scale_factor(self) -> int:
        return self.right_x - self.left_x

    def get_square_centre_coords(self, index: int) -> tuple[int, int]:
        a1_x_centre = self.left_x + self.scale_factor() * 0.037
        a1_y_centre = self.y + self.scale_factor() * 0.053
        field_size_px = self.scale_factor() * 0.072

        x = int(a1_x_centre + ((index % 8) * field_size_px))
        y = int(a1_y_centre + ((index // 8) * field_size_px))
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


class WatchMode(BaseMode):
    def __init__(self, args: Arguments) -> None:
        self.screenshot: Image
        self.prev_coords: Optional[FlyOrDieWatchCoords] = None
        self.prev_board = Board.empty()
        self.prev_unknown_squares: set[int] = set()

    def get_board(self) -> Board:
        try:
            coords, board, unknown_squares = self._get_board()
        except BoardNotFound:
            return self.prev_board

        self.prev_coords = coords
        self.prev_unknown_squares = unknown_squares
        self.prev_board = board
        return board

    def get_ui_details(self) -> dict[str, Any]:
        return {"unknown_squares": self.prev_unknown_squares}

    def _get_board(self) -> tuple[FlyOrDieWatchCoords, Board, set[int]]:
        self.screenshot = pyautogui.screenshot()

        if self.prev_coords:
            try:
                board, unknown_squares = self.get_board_from_coords(self.prev_coords)
            except BoardNotFound:
                if self.prev_coords.is_valid_coords(self.screenshot):
                    # No board is visible, but anchor coords are still the same.
                    # This prevents a full re-search of the screenshot for anchors.
                    raise BoardNotFound
            else:
                return self.prev_coords, board, unknown_squares

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
                    board, unknown_squares = self.get_board_from_coords(coords)
                    return coords, board, unknown_squares

        raise BoardNotFound

    def get_board_from_coords(
        self, coords: FlyOrDieWatchCoords
    ) -> tuple[Board, set[int]]:
        squares = [EMPTY] * 64
        unknown_squares: set[int] = set()

        for index in range(64):
            centre = coords.get_square_centre_coords(index)

            try:
                square = self.get_square_at_coords(centre)
            except UnknownSquare:
                unknown_squares.add(index)
                continue

            squares[index] = square

        turn = self.get_turn(coords)
        board = Board.from_squares(squares, turn)
        return board, unknown_squares

    def get_square_at_coords(self, coord: tuple[int, int]) -> int:
        pixel = self.screenshot.getpixel(coord)

        for empty in FOD_EMPTIES:
            if is_similar_color(pixel, empty, 20):
                return EMPTY

        if is_similar_color(pixel, FOD_BLACK, 20):
            return BLACK

        if is_similar_color(pixel, FOD_WHITE, 20):
            return WHITE

        print(pixel)

        raise UnknownSquare

    def get_turn(self, coords: FlyOrDieWatchCoords) -> int:
        turn_highlighter_coords = coords.get_turn_highlighter_coords()
        turn_highlighter = self.screenshot.getpixel(turn_highlighter_coords)

        turn_color_coords = coords.get_turn_color_coords()

        try:
            turn = self.get_square_at_coords(turn_color_coords)
        except UnknownSquare:
            raise BoardNotFound

        if is_similar_color(turn_highlighter, FOD_TURN_HIGHLIGHTER, 50):
            return turn
        return opponent(turn)
