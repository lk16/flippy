import typer
from typing import Annotated

from flippy.config import PgnConfig
from flippy.mode.training.exercise_list import BLACK_TREE, WHITE_TREE, Node
from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.game import Game
from flippy.othello.position import NormalizedPosition, Position

app = typer.Typer(pretty_exceptions_enable=False)


class ChoiceInfo:
    def __init__(self, moves: list[int], in_training: bool) -> None:
        self.moves = moves
        self.wins = 0
        self.losses = 0
        self.draws = 0
        self.mistakes = 0  # TODO use
        self.in_training = in_training

    def games(self) -> int:
        return self.wins + self.losses + self.draws


def get_best_choices(color: int) -> dict[NormalizedPosition, NormalizedPosition]:
    """Returns mapping from position to best child position according to the training tree"""

    def best_choices(
        position: Position, node: Node
    ) -> dict[NormalizedPosition, NormalizedPosition]:
        choices = {}

        moves = node.moves.split()

        if len(moves) < 2:
            child = position

            if len(moves) == 1:
                first_move = moves[0]
                child = position.do_move(Board.field_to_index(first_move))

            for child_node in node.children:
                choices.update(best_choices(child, child_node))

            return choices

        first_move, second_move = moves[:2]  # Take first two moves safely

        child = position.do_move(Board.field_to_index(first_move))
        grand_child = child.do_move(Board.field_to_index(second_move))

        if not node.children:
            return {child.normalized(): grand_child.normalized()}

        for child_node in node.children:
            choices.update(best_choices(grand_child, child_node))

        return choices

    if color == WHITE:
        return best_choices(Position.start(), WHITE_TREE)

    else:
        return best_choices(Position.start(), BLACK_TREE)


@app.command()
def training_analysis(
    player: Annotated[str, typer.Argument(help="Player color (white/black)")],
    game_count: Annotated[
        int, typer.Option("--games", "-g", help="Number of games to analyze")
    ],
    top_n: Annotated[
        int, typer.Option("--top", "-t", help="Number of top positions to display")
    ],
    min_games: Annotated[
        int,
        typer.Option(
            "--min-games",
            "-m",
            help="Minimum games for a position to be considered",
        ),
    ] = 1,
    sort_by: Annotated[
        str,
        typer.Option("--sort", "-s", help="Sort by: games, winrate, mistakes"),
    ] = "games",
) -> None:
    """Analyze game positions against training exercises and show statistics for common positions"""
    if player.lower() == "white":
        color = WHITE
    elif player.lower() == "black":
        color = BLACK
    else:
        raise ValueError(f"Invalid player: {player}. Must be 'white' or 'black'")

    pgn_config = PgnConfig()
    prefix = pgn_config.target_folder / "normal"

    best_choices = get_best_choices(color)
    choices: dict[NormalizedPosition, ChoiceInfo] = {}

    games: list[Game] = []

    print(f"Loading up to {game_count} games for {player}...")
    for file in sorted(prefix.rglob("*.pgn"), reverse=True):
        game = Game.from_pgn(file)

        if game.get_color_any(list(pgn_config.all_usernames)) == color:
            games.append(game)

        if len(games) >= game_count:
            break

    print(f"Analyzing {len(games)} games...")
    for game in games:
        for i, board in enumerate(game.boards):
            if board.turn == color:
                normalized = board.position.normalized()

                if normalized not in choices:
                    in_training = normalized in best_choices
                    choices[normalized] = ChoiceInfo(game.moves[:i], in_training)

                winner = game.get_winner()
                if winner is None:
                    choices[normalized].draws += 1
                elif winner == color:
                    choices[normalized].wins += 1
                else:
                    choices[normalized].losses += 1

                if i < len(game.boards) - 1:
                    next_normalized = game.boards[i + 1].position.normalized()

                    if (
                        normalized in best_choices
                        and best_choices[normalized] != next_normalized
                    ):
                        choices[normalized].mistakes += 1

    print_choices_sorted(choices, top_n, min_games, sort_by)


def print_choices_sorted(
    choices: dict[NormalizedPosition, ChoiceInfo],
    top_n: int,
    min_games: int = 1,
    sort_by: str = "games",
) -> None:
    """Print positions sorted by specified criteria"""
    # Filter positions with minimum number of games
    filtered_choices = [c for c in choices.values() if c.games() >= min_games]

    # Sort based on user preference
    def get_winrate_key(item: ChoiceInfo) -> tuple[float, int]:
        return (item.wins / item.games() if item.games() > 0 else 0, item.games())

    def get_mistakes_key(item: ChoiceInfo) -> tuple[float, int]:
        return (
            item.mistakes / item.games()
            if item.games() > 0 and item.in_training
            else 0,
            item.games(),
        )

    def get_games_key(item: ChoiceInfo) -> tuple[int, int]:
        return (item.games(), -len(item.moves))

    if sort_by.lower() == "winrate":
        key_func = get_winrate_key
    elif sort_by.lower() == "mistakes":
        key_func = get_mistakes_key
    else:  # Default to games
        key_func = get_games_key

    sorted_choices = sorted(filtered_choices, key=key_func, reverse=True)[:top_n]

    # Print header
    print(f"\n{'Games':>5} | {'Win Rate':>8} | {'Correct':>8} | {'In Tree':>7} | Moves")
    print("-" * 70)

    for choice in sorted_choices:
        count = choice.games()
        moves = Board.indexes_to_fields(choice.moves)
        win_rate = choice.wins / count if count > 0 else 0

        if choice.in_training:
            correct_rate = (
                f"{(choice.games() - choice.mistakes) / choice.games():7.2%}"
                if choice.games() > 0
                else "   0.00%"
            )
            in_tree = "Yes"
        else:
            correct_rate = "    ---"
            in_tree = "No"

        print(
            f"{count:>5} |  {win_rate:>7.2%} |  {correct_rate} | {in_tree:>7} | {moves}"
        )


if __name__ == "__main__":
    app()
