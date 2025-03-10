import typer
from typing import Annotated

from flippy.config import PgnConfig
from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.game import Game
from flippy.othello.position import NormalizedPosition

app = typer.Typer(pretty_exceptions_enable=False)


GAME_COUNT = 1000


class ChoiceInfo:
    def __init__(self, moves: list[int]) -> None:
        self.moves = moves
        self.wins = 0
        self.losses = 0
        self.draws = 0
        self.mistakes = 0  # TODO use

    def games(self) -> int:
        return self.wins + self.losses + self.draws


@app.command()
def tree_priority(
    player: str,
    game_count: Annotated[int, typer.Option("--games")],
    top_n: Annotated[int, typer.Option("--top")],
) -> None:
    if player == "white":
        color = WHITE
    elif player == "black":
        color = BLACK
    else:
        raise ValueError(f"Invalid player: {player}")

    pgn_config = PgnConfig()

    prefix = pgn_config.target_folder / "normal"

    white_choices: dict[NormalizedPosition, ChoiceInfo] = {}
    _ = white_choices  # TODO use

    black_choices: dict[NormalizedPosition, ChoiceInfo] = {}

    games: list[Game] = []

    for file in sorted(prefix.rglob("*.pgn"), reverse=True):
        game = Game.from_pgn(file)

        if game.get_color_any(list(pgn_config.all_usernames)) == color:
            games.append(game)

        if len(games) >= game_count:
            break

    for game in games:
        for i, board in enumerate(game.boards):
            if board.turn == color:
                normalized = board.position.normalized()

                if normalized not in black_choices:
                    black_choices[normalized] = ChoiceInfo(game.moves[: i + 1])

                winner = game.get_winner()
                if winner is None:
                    black_choices[normalized].draws += 1
                elif winner == color:
                    black_choices[normalized].wins += 1
                else:
                    black_choices[normalized].losses += 1

    print_choices_sorted(black_choices, top_n)


def print_choices_sorted(
    choices: dict[NormalizedPosition, ChoiceInfo], top_n: int
) -> None:
    for choice in sorted(
        choices.values(),
        key=lambda item: (item.games(), -len(item.moves)),
        reverse=True,
    )[:top_n]:
        count = choice.games()
        moves = Board.indexes_to_fields(choice.moves)
        win_rate = choice.wins / count
        print(f"{count:>4} | {win_rate:>7.2%} | {moves}")


if __name__ == "__main__":
    app()
