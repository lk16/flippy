from __future__ import annotations

from copy import copy
from datetime import date, datetime
from pathlib import Path
from typing import Iterable, Optional

from flippy.othello.board import BLACK, PASS_MOVE, WHITE, Board, InvalidMove


class Game:
    def __init__(self, file: Optional[Path] = None) -> None:
        self.file = file
        self.metadata: dict[str, str] = {}
        self.boards: list[Board] = []
        self.moves: list[int] = []

    @classmethod
    def from_pgn(cls, file: Path) -> Game:
        contents = file.read_text(errors="ignore")
        game = cls.from_string(contents)
        game.file = file
        return game

    @classmethod
    def from_string(cls, string: str) -> Game:
        game = Game()

        lines = string.split("\n")
        for line_offset, line in enumerate(lines):
            if not line.startswith("["):
                break

            split_line = line.split(" ")
            key = split_line[0][1:]
            value = split_line[1][1:-2]
            game.metadata[key] = value

        board = Board.start()
        game.boards.append(copy(board))

        for line in lines[line_offset:]:
            if line == "":
                continue

            for word in line.split(" "):
                if word[0].isdigit():
                    continue

                move = Board.field_to_index(word)

                try:
                    board.do_move(move)
                except InvalidMove:
                    # Some PGN's don't mark passed moves properly
                    board = board.do_move(PASS_MOVE)
                    game.boards.append(board)

                board = board.do_move(move)
                game.moves.append(move)
                game.boards.append(board)

        return game

    def is_xot(self) -> bool:
        try:
            variant = self.metadata["Variant"]
        except KeyError:
            return False

        return variant == "xot"

    def get_date(self) -> date:
        return datetime.strptime(self.metadata["Date"], "%Y.%m.%d").date()

    def get_datetime(self) -> Optional[datetime]:
        try:
            raw = self.metadata["Date"] + " " + self.metadata["Time"]
        except KeyError:
            return None
        return datetime.strptime(raw, "%Y.%m.%d %H:%M:%S")

    def get_white_player(self) -> str:
        return self.metadata["White"]

    def get_black_player(self) -> str:
        return self.metadata["Black"]

    def get_winning_player(self) -> Optional[str]:
        winner = self.get_winner()
        if winner is None:
            return None
        elif winner == WHITE:
            return self.get_white_player()
        else:
            return self.get_black_player()

    def get_color(self, username: str) -> Optional[int]:
        if username == self.get_white_player():
            return WHITE
        if username == self.get_black_player():
            return BLACK
        return None

    def get_color_any(self, usernames: Iterable[str]) -> Optional[int]:
        for username in usernames:
            color = self.get_color(username)
            if color is not None:
                return color
        return None

    def get_winner(self) -> Optional[int]:
        if self.metadata["Result"] == "1/2-1/2":
            return None

        black, white = [int(n) for n in self.metadata["Result"].split("-")]

        if black > white:
            return BLACK
        if white > black:
            return WHITE
        return None

    def get_black_score(self) -> int:
        if self.metadata["Result"] == "1/2-1/2":
            return 0

        black, white = [int(n) for n in self.metadata["Result"].split("-")]

        if white == black:
            return 0
        elif black > white:
            return 64 - 2 * white
        else:
            return -64 + 2 * black

    def zip_board_moves(self) -> zip[tuple[Board, int]]:
        return zip(self.boards[:-1], self.moves, strict=True)

    def get_all_children(self) -> list[Board]:
        all_children: list[Board] = []

        for board in self.boards:
            for child in board.get_children():
                all_children.append(child)

        return all_children
