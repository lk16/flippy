from copy import copy
from pathlib import Path
from typing import Dict, List

from flippy.othello.board import Board


class Game:
    def __init__(self) -> None:
        self.metadata: Dict[str, str] = {}
        self.boards: List[Board] = []
        self.moves: List[int] = []

    @classmethod
    def from_pgn(cls, file: Path) -> "Game":
        contents = file.read_text()

        game = Game()

        lines = contents.split("\n")
        for offset, line in enumerate(lines):
            if not line.startswith("["):
                break

            split_line = line.split(" ")
            key = split_line[0][1:]
            value = split_line[1][1:-2]
            game.metadata[key] = value

        board = Board.start()
        game.boards.append(copy(board))

        for line in lines[offset:]:
            if line == "":
                continue

            for word in line.split(" "):
                if word[0].isdigit():
                    continue

                move = Board.str_to_offset(word)
                game.moves.append(move)
                child = board.do_move(move)

                assert child
                game.boards.append(child)
                board = child

        return game