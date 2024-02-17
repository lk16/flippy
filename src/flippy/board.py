ROWS, COLS = 8, 8


class Board:
    def __init__(self) -> None:
        self.squares = [[0] * COLS for _ in range(ROWS)]
        self.squares[3][3] = self.squares[4][4] = 1  # Initial black pieces
        self.squares[3][4] = self.squares[4][3] = -1  # Initial white pieces
        self.turn = -1

    def is_valid_move(self, row: int, col: int) -> bool:
        return self.__is_valid_move(row, col, self.turn)

    def __is_valid_move(self, row: int, col: int, color: int) -> bool:
        if self.squares[row][col] != 0:
            return False

        for dr in range(-1, 2):
            for dc in range(-1, 2):
                if dr == 0 and dc == 0:
                    continue
                r, c = row + dr, col + dc
                if 0 <= r < ROWS and 0 <= c < COLS and self.squares[r][c] == -color:
                    while (
                        0 <= r < ROWS and 0 <= c < COLS and self.squares[r][c] == -color
                    ):
                        r, c = r + dr, c + dc
                    if 0 <= r < ROWS and 0 <= c < COLS and self.squares[r][c] == color:
                        return True
        return False

    def do_move(self, row: int, col: int) -> None:
        if not self.is_valid_move(row, col):
            return

        self.squares[row][col] = self.turn

        for dr in range(-1, 2):
            for dc in range(-1, 2):
                if dr == 0 and dc == 0:
                    continue
                r, c = row + dr, col + dc
                if 0 <= r < ROWS and 0 <= c < COLS and self.squares[r][c] == -self.turn:
                    flip_list = []
                    while (
                        0 <= r < ROWS
                        and 0 <= c < COLS
                        and self.squares[r][c] == -self.turn
                    ):
                        flip_list.append((r, c))
                        r, c = r + dr, c + dc
                    if (
                        0 <= r < ROWS
                        and 0 <= c < COLS
                        and self.squares[r][c] == self.turn
                    ):
                        for flip_row, flip_col in flip_list:
                            self.squares[flip_row][flip_col] = self.turn

        if self.__has_moves(-self.turn):
            self.turn *= -1

    def has_moves(self) -> bool:
        return self.__has_moves(self.turn)

    def __has_moves(self, color: int) -> bool:
        for row in range(ROWS):
            for col in range(COLS):
                if self.__is_valid_move(row, col, color):
                    return True
        return False

    def is_game_end(self) -> bool:
        return not self.__has_moves(self.turn) and not self.__has_moves(-self.turn)
