import struct
from io import BufferedReader
from pathlib import Path
from typing import cast

from flippy.othello.game import Game


class Wthor:
    def __init__(self, file: Path) -> None:
        self.file = file
        self.games = self._parse()

    def _parse(self) -> list[Game]:
        with open(self.file, "rb") as f:
            game_count = self._parse_header(f)
            games = [self._parse_game(f) for _ in range(game_count)]

        return games

    def _parse_header(self, f: BufferedReader) -> int:
        file_header = f.read(16)

        (
            _,  # century
            _,  # year
            _,  # month
            _,  # day
            game_count,
            _,  # game_count_2
            _,  # games_year
            _,  # param_p1
            _,  # param_p2
            _,  # param_p3
            _,  # reserved
        ) = struct.unpack("<BBBBIHHBBBB", file_header)

        return cast(int, game_count)

    def _parse_game(self, f: BufferedReader) -> Game:
        game_record = f.read(68)

        (
            _,  # tournament_id,
            _,  # player_black,
            _,  # player_white,
            _,  # black_score_real,
            _,  # black_score_theoretical,
        ) = struct.unpack("<HHHBB", game_record[:8])
        move_bytes = game_record[8:]

        moves: list[int] = []

        # Read and print moves
        for move_byte in move_bytes:
            if move_byte == 0:
                break  # Assuming 0 signifies end of moves
            row = move_byte // 10
            col = move_byte % 10
            move = (row - 1) * 8 + (col - 1)

            moves.append(move)

        return Game.from_moves(moves)

    def get_games(self) -> list[Game]:
        return self.games
