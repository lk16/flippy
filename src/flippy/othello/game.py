from __future__ import annotations

from copy import copy
from datetime import date, datetime
from pathlib import Path
from typing import Dict, Iterable, List, Optional

from flippy.othello.board import BLACK, PASS_MOVE, WHITE, Board, InvalidMove


class Game:
    def __init__(self, file: Optional[Path] = None) -> None:
        self.file = file
        self.metadata: Dict[str, str] = {}
        self.boards: List[Board] = []
        self.moves: List[int] = []

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

    def get_color(self, username: str) -> Optional[int]:
        if self.metadata["White"] == username:
            return WHITE
        if self.metadata["Black"] == username:
            return BLACK
        return None

    def get_color_any(self, usernames: Iterable[str]) -> Optional[int]:
        for username in usernames:
            color = self.get_color(username)
            if color is not None:
                return color
        return None

    def get_winner(self) -> Optional[int]:
        final_position = self.boards[-1]
        black_discs = final_position.count(BLACK)
        white_discs = final_position.count(WHITE)

        if black_discs > white_discs:
            return BLACK
        if white_discs > black_discs:
            return WHITE
        return None
