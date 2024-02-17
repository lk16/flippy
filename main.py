import os
import sys

os.environ["PYGAME_HIDE_SUPPORT_PROMPT"] = "hide"

import pygame  # noqa:E402

# Constants
WIDTH, HEIGHT = 400, 400
ROWS, COLS = 8, 8
SQUARE_SIZE = WIDTH // COLS

# Colors
WHITE = (255, 255, 255)
BLACK = (0, 0, 0)
GREEN = (0, 128, 0)

# Initialize Pygame
pygame.init()

# Create the game window
screen = pygame.display.set_mode((WIDTH, HEIGHT))
pygame.display.set_caption("Othello")

# Initialize the game board
board = [[0] * COLS for _ in range(ROWS)]
board[3][3] = board[4][4] = 1  # Initial black pieces
board[3][4] = board[4][3] = -1  # Initial white pieces


# Functions
def draw_board() -> None:
    screen.fill(GREEN)
    for row in range(ROWS):
        for col in range(COLS):
            pygame.draw.rect(
                screen,
                GREEN,
                (col * SQUARE_SIZE, row * SQUARE_SIZE, SQUARE_SIZE, SQUARE_SIZE),
            )
            if board[row][col] == 1:
                pygame.draw.circle(
                    screen,
                    WHITE,
                    (
                        col * SQUARE_SIZE + SQUARE_SIZE // 2,
                        row * SQUARE_SIZE + SQUARE_SIZE // 2,
                    ),
                    SQUARE_SIZE // 2 - 5,
                )
            elif board[row][col] == -1:
                pygame.draw.circle(
                    screen,
                    BLACK,
                    (
                        col * SQUARE_SIZE + SQUARE_SIZE // 2,
                        row * SQUARE_SIZE + SQUARE_SIZE // 2,
                    ),
                    SQUARE_SIZE // 2 - 5,
                )


def is_valid_move(row: int, col: int, player: int) -> bool:
    if board[row][col] != 0:
        return False

    for dr in range(-1, 2):
        for dc in range(-1, 2):
            if dr == 0 and dc == 0:
                continue
            r, c = row + dr, col + dc
            if 0 <= r < ROWS and 0 <= c < COLS and board[r][c] == -player:
                while 0 <= r < ROWS and 0 <= c < COLS and board[r][c] == -player:
                    r, c = r + dr, c + dc
                if 0 <= r < ROWS and 0 <= c < COLS and board[r][c] == player:
                    return True
    return False


def make_move(row: int, col: int, player: int) -> bool:
    if not is_valid_move(row, col, player):
        return False

    board[row][col] = player

    for dr in range(-1, 2):
        for dc in range(-1, 2):
            if dr == 0 and dc == 0:
                continue
            r, c = row + dr, col + dc
            if 0 <= r < ROWS and 0 <= c < COLS and board[r][c] == -player:
                flip_list = []
                while 0 <= r < ROWS and 0 <= c < COLS and board[r][c] == -player:
                    flip_list.append((r, c))
                    r, c = r + dr, c + dc
                if 0 <= r < ROWS and 0 <= c < COLS and board[r][c] == player:
                    for flip_row, flip_col in flip_list:
                        board[flip_row][flip_col] = player
    return True


def get_winner() -> str:
    white_count = sum(row.count(1) for row in board)
    black_count = sum(row.count(-1) for row in board)
    if white_count > black_count:
        return "White"
    elif white_count < black_count:
        return "Black"
    else:
        return "Draw"


# Main game loop
current_player = -1  # Black starts
running = True

while running:
    for event in pygame.event.get():
        if event.type == pygame.QUIT:
            running = False
        elif event.type == pygame.MOUSEBUTTONDOWN:
            mouseX, mouseY = event.pos
            clicked_col = mouseX // SQUARE_SIZE
            clicked_row = mouseY // SQUARE_SIZE

            if 0 <= clicked_row < ROWS and 0 <= clicked_col < COLS:
                if make_move(clicked_row, clicked_col, current_player):
                    current_player *= -1  # Switch player

    draw_board()
    pygame.display.flip()

    # Check for game over
    if all(all(cell != 0 for cell in row) for row in board):
        winner = get_winner()
        print(f"Game over! {winner} wins!")
        running = False

# Quit Pygame
pygame.quit()
sys.exit()
