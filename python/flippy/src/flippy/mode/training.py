from typing import Any

from flippy.arguments import Arguments
from flippy.mode.game import GameMode
from flippy.othello.board import Board
from flippy.othello.position import InvalidMove
from flippy.training.file import DEFAULT_TRAINING_SCORE_FILE_PATH, Training


class TrainingMode(GameMode):
    def __init__(self, args: Arguments) -> None:
        super().__init__(args)

        self.training = Training(DEFAULT_TRAINING_SCORE_FILE_PATH)

        self.board = self.training.get_random_start_position()
        self.wrong_moves: set[int] = set()

    def get_board(self) -> Board:
        return self.board

    def on_move(self, move: int) -> None:
        board = self.get_board()

        try:
            child = board.do_move(move)
        except InvalidMove:
            return

        if not self.training.is_best_move(board, child):
            if not self.wrong_moves:
                self.training.adjust_scores(board, False, 1.0)

            self.wrong_moves.add(move)
            return

        if not self.wrong_moves:
            self.training.adjust_scores(board, True, 1.0)

        self.wrong_moves.clear()

        next_board = self.training.get_random_child(child)

        if not next_board:
            # Next board not in training file.
            self.board = self.training.get_random_start_position()
            return

        self.board = next_board

    def get_ui_details(self) -> dict[str, Any]:
        return {
            "wrong_moves": self.wrong_moves,
        }
