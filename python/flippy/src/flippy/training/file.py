from __future__ import annotations

import json
import random
from pathlib import Path

from flippy import PROJECT_ROOT
from flippy.book import MAX_SAVABLE_DISCS, MIN_LEARN_LEVEL
from flippy.book.api_client import APIClient
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxEvaluations, EdaxRequest
from flippy.othello.board import BLACK, WHITE, Board, opponent
from flippy.othello.position import NormalizedPosition, Position

DEFAULT_TRAINING_SCORE_FILE_PATH = PROJECT_ROOT / ".flippy/training_scores.json"

DEFAULT_RATING = 1200

MAX_TRAINING_DISCS = 16


class Training:
    def __init__(self, score_path: Path) -> None:
        self.score_path = score_path
        self.api_client = APIClient()
        self.evaluations = EdaxEvaluations()

        try:
            data = json.loads(score_path.read_text())
        except FileNotFoundError:
            data = {
                "player_rating": DEFAULT_RATING,
                "positions": {},
            }

        self.player_rating: int = data["player_rating"]
        self.positions: dict[NormalizedPosition, int] = {
            NormalizedPosition.from_api(k): v for k, v in data["positions"].items()
        }

    def save(self) -> None:
        data = {
            "player_rating": self.player_rating,
            "positions": {k.to_api(): v for k, v in self.positions.items()},
        }
        self.score_path.write_text(json.dumps(data, indent=2))

    def get_random_start_position(self) -> Board:
        black_start = Position.start().normalized()
        white_start = Position.start().do_move(19).normalized()

        if random.randint(0, 1) == 0:
            position = black_start.to_position()
            color = BLACK
        else:
            position = white_start.to_position()
            color = WHITE
            position = position.rotated(random.choice([0, 3, 4, 7]))

        return Board(position, color)

    def get_random_child(self, board: Board) -> Board | None:
        """
        Returns a weighted random child position if configuration allows, otherwise None.
        """
        if board.count_discs() + 1 > MAX_TRAINING_DISCS:
            return None

        normalized_children = board.position.get_normalized_children()

        # Get ratings for all normalized children, using default if not found
        child_ratings = {
            child: self.positions.get(child, DEFAULT_RATING)
            for child in normalized_children
        }

        if not child_ratings:
            return None

        self._load_child_evaluations(board)

        scores = sorted(
            set(-self.evaluations[child].score for child in normalized_children),
            reverse=True,
        )

        def weight(pos: NormalizedPosition) -> float:
            score_weight = 500 * (0.2 ** scores.index(-self.evaluations[pos].score))

            rating = self.positions.get(pos, DEFAULT_RATING)

            rating_weight = rating - DEFAULT_RATING

            return min(500, max(10, score_weight + rating_weight))

        # Weight probabilities by rating - higher rated positions are more likely
        total_rating = sum([weight(pos) for pos in child_ratings.keys()])
        normalized_weights = [
            weight(pos) / total_rating for pos in child_ratings.keys()
        ]

        # Print table header
        print("\nPosition selection probabilities:")
        print("-" * 125)
        print(
            f"{'Position':40} {'Rating':>10} {'Eval':>10} {'Score W':>10} {'Rating W':>10} {'Total W':>10} {'Chance %':>15}"
        )
        print("-" * 125)

        # Calculate and print each position's details sorted by total weight
        position_details = []
        for pos, rating in child_ratings.items():
            pos_str = pos.to_api()
            rating_str = str(rating) if rating != DEFAULT_RATING else "-"
            eval_str = str(-self.evaluations[pos].score)
            score_w = 500 * (0.3 ** scores.index(-self.evaluations[pos].score))
            rating_w = rating - DEFAULT_RATING
            total_w = min(500, max(10, score_w + rating_w))
            chance = (weight(pos) / total_rating) * 100
            position_details.append(
                (pos_str, rating_str, eval_str, score_w, rating_w, total_w, chance)
            )

        # Sort by total weight descending
        position_details.sort(key=lambda x: x[5], reverse=True)

        for details in position_details:
            print(
                f"{details[0]:40} {details[1]:>10} {details[2]:>10} {details[3]:>10.1f} {details[4]:>10.1f} {details[5]:>10.1f} {details[6]:>14.1f}"
            )
        print("-" * 125)
        print()

        # Select a normalized child position based on weighted probabilities
        normalized_child = random.choices(
            list(child_ratings.keys()), weights=normalized_weights, k=1
        )[0]

        children = board.get_children()

        for rot in range(8):
            child_position = normalized_child.to_position().rotated(rot)
            child_board = Board(child_position, opponent(board.turn))

            if child_board in children:
                return child_board

        raise ValueError("No rotation matches child position")  # Should never happen.

    def _load_child_evaluations(self, board: Board) -> None:
        children = board.position.get_normalized_children()

        missing_children = self.evaluations.get_missing(children)

        if missing_children:
            self.evaluations.update(self.api_client.lookup_positions(missing_children))

        child_disc_count = board.count_discs() + 1
        if child_disc_count > MAX_SAVABLE_DISCS:
            raise ValueError(
                f"Child with {child_disc_count} discs won't be found or saved in DB."
            )

        missing_children = self.evaluations.get_missing_children(board.position)

        # Do not compute missing children all the way, because it takes too long to do during training.
        edax_request = EdaxRequest(
            positions=missing_children, level=MIN_LEARN_LEVEL, source=None
        )
        new_evaluations: EdaxEvaluations = start_evaluation_sync(edax_request)
        self.api_client.save_learned_evaluations(new_evaluations.values())
        self.evaluations.update(new_evaluations)

    def is_best_move(self, board: Board, next_board: Board) -> bool:
        children = board.position.get_normalized_children()

        self._load_child_evaluations(board)

        # Find lowest score for opponent, so best move for us.
        min_score = min(self.evaluations[child].score for child in children)
        best_moves: set[NormalizedPosition] = set(
            child for child in children if self.evaluations[child].score == min_score
        )

        return next_board.position.normalized() in best_moves

    def adjust_scores(self, board: Board, correct: bool, multiplier: float) -> None:
        position = board.position.normalized()

        player_rating = self.player_rating
        position_rating = self.positions.get(position, DEFAULT_RATING)

        # Calculate Elo-style rating adjustment
        expected_score = 1 / (1 + 10 ** ((position_rating - player_rating) / 400))
        actual_score = 1.0 if correct else 0.0

        # Calculate base adjustment and ensure minimum of 1
        base_adjustment = 32 * (actual_score - expected_score) * multiplier
        adjustment = max(1, int(abs(base_adjustment)))

        # Apply adjustment with correct sign
        if correct:
            self.player_rating += adjustment
            self.positions[position] = position_rating - adjustment
        else:
            self.player_rating -= adjustment
            self.positions[position] = position_rating + adjustment

        print(f"Adjusted by {adjustment}")
        print(f"Position rating: {position_rating}")
        print(f"Player rating: {self.player_rating}")
        print()

        self.save()

    def show(self) -> None:
        positions = sorted(self.positions.items(), key=lambda x: x[1], reverse=True)

        for position, rating in positions:
            print(f"{position.to_api()} {rating}")

        print()
        print(f"Rated positions: {len(positions)}")
        print()
        print(f"Player rating: {self.player_rating}")
