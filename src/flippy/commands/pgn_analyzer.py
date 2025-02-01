import requests
import typer
from pathlib import Path
from typing import Annotated

from flippy.book import is_savable_evaluation
from flippy.book.models import SerializedEvaluation
from flippy.config import get_book_server_token, get_book_server_url
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxEvaluations, EdaxRequest
from flippy.othello.board import BLACK, WHITE, Board
from flippy.othello.game import Game
from flippy.othello.position import PASS_MOVE, Position


class PgnAnanlyzer:
    def __init__(self, file: Path, level: int) -> None:
        self.file = file
        self.level = level
        self.game = Game.from_pgn(file)
        self.evaluations = EdaxEvaluations()
        self.server_url = get_book_server_url()
        self.token = get_book_server_token()

    def _get_best(self, board: Board) -> tuple[list[int], int]:
        child_scores: list[tuple[int, int]] = []

        for move in board.get_moves_as_set():
            child = board.do_move(move)
            try:
                evaluation = self.evaluations.lookup(child.position)
            except KeyError:
                # Fetch from API if not found locally
                response = requests.get(
                    f"{self.server_url}/api/positions",
                    json=[child.position.to_api()],
                    headers={"x-token": self.token},
                )
                response.raise_for_status()
                server_evaluations = [
                    SerializedEvaluation.model_validate(item)
                    for item in response.json()
                ]
                if server_evaluations:
                    evaluation = server_evaluations[0].to_evaluation()
                    self.evaluations.add(child.position, evaluation)
                else:
                    continue

            child_scores.append((move, evaluation.score))

        if not child_scores:
            return [], 0

        # Take minimum, because scores are from opponent's point of view
        best_score = min(score for _, score in child_scores)
        best_moves = [move for move, score in child_scores if score == best_score]

        return best_moves, best_score

    def _get_all_positions(self) -> set[Position]:
        positions: set[Position] = set()
        for board in self.game.boards:
            positions.add(board.position)
            for child in board.get_children():
                positions.add(child.position)
        return positions

    def _evaluate_positions(self, all_positions: set[Position]) -> EdaxEvaluations:
        request_positions = self.evaluations.get_missing(all_positions)
        request = EdaxRequest(request_positions, self.level, source=None)
        return start_evaluation_sync(request)

    def _get_colored_score(self, score: int, board: Board) -> str:
        if score == 0:
            return "◑ + 0"

        if (score > 0) != (board.turn == BLACK):
            return f"○ +{abs(score):>2}"

        return f"● +{abs(score):>2}"

    def _get_move_evaluation_line(self, move_offset: int) -> str:
        board = self.game.boards[move_offset]
        played_move = self.game.moves[move_offset]

        turn = {WHITE: "●", BLACK: "○"}[board.turn]
        possible_moves = board.get_moves_as_set()

        if not possible_moves:
            assert played_move == PASS_MOVE
            return f"{move_offset+1:>2}. {turn} --"

        best_moves, best_score = self._get_best(board)
        best_score_str = self._get_colored_score(best_score, board)
        best_fields = ",".join(Position.index_to_field(move) for move in best_moves)

        played_field = Position.index_to_field(played_move)
        played_child = board.do_move(played_move)

        score = self.evaluations.lookup(played_child.position).score
        score_str = self._get_colored_score(score, board)

        output_line = f"{move_offset+1:>2}. {turn} {played_field} {score_str}"

        if played_move not in best_moves:
            output_line += f" (best: {best_score_str} @ {best_fields})"

        return output_line

    def __call__(self) -> None:
        # Get set of all positions and their children in game
        all_positions = self._get_all_positions()

        # Fetch evaluations from server API in batches of 100
        server_evaluations: list[SerializedEvaluation] = []
        positions_list = list(all_positions)
        for i in range(0, len(positions_list), 100):
            batch = positions_list[i : i + 100]
            response = requests.get(
                f"{self.server_url}/api/positions",
                json=[pos.to_api() for pos in batch],
                headers={"x-token": self.token},
            )
            response.raise_for_status()
            server_evaluations.extend(
                SerializedEvaluation.model_validate(item) for item in response.json()
            )

        evaluations = {
            Position.from_api(e.position): e.to_evaluation() for e in server_evaluations
        }
        self.evaluations.update(evaluations)

        # Compute evaluations for missing positions
        computed_evaluations = self._evaluate_positions(all_positions)
        self.evaluations.update(computed_evaluations.values)

        # Submit newly computed evaluations to server API
        payload = [
            SerializedEvaluation.from_evaluation(eval).model_dump()
            for eval in computed_evaluations.values.values()
            if is_savable_evaluation(eval)
        ]
        if payload:
            response = requests.post(
                f"{self.server_url}/api/evaluations",
                json=payload,
                headers={"x-token": self.token},
            )
            response.raise_for_status()

        for move_offset in range(len(self.game.moves)):
            output_line = self._get_move_evaluation_line(move_offset)
            print(output_line)


app = typer.Typer(pretty_exceptions_enable=False)


@app.command()
def pgn_analyzer(file: Path, level: Annotated[int, typer.Option("-l")] = 18) -> None:
    PgnAnanlyzer(file, level)()


if __name__ == "__main__":
    app()
