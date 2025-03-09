from __future__ import annotations

import json
import random
from typing import Any

from flippy import PROJECT_ROOT
from flippy.arguments import Arguments
from flippy.mode.base import BaseMode
from flippy.mode.training.exercise import Exercise
from flippy.mode.training.exercise_list import get_exercises
from flippy.othello.board import Board

EXERCISE_SCORES_PATH = PROJECT_ROOT / ".flippy/exercise_scores.json"

EXERCISE_DEFAULT_SCORE = 100
EXERCISE_MIN_SCORE = 10

EXERCISE_CORRECT_DIFF = 30
EXERCISE_INCORRECT_DIFF = 40

EXERCISE_WEIGHT_POWER = 2.0


class NoExercisesLeft(Exception):
    pass


class TrainingMode(BaseMode):
    def __init__(self, _: Arguments) -> None:
        self.exercises = get_exercises()
        self.move_mistakes: set[int] = set()
        self.exercise_mistakes = False
        self.current_exercise_id = 0

        self.exercise_scores: dict[str, int] = {}

        # Load existing exercise scores from file if available
        if EXERCISE_SCORES_PATH.exists():
            self.exercise_scores = json.load(EXERCISE_SCORES_PATH.open())

        # Remove exercises that are no longer in the exercise list.
        exercise_keys = {exercise.raw for exercise in self.exercises}
        self.exercise_scores = {
            ex_id: score
            for ex_id, score in self.exercise_scores.items()
            if ex_id in exercise_keys
        }

        # Initialize default scores for any new exercises
        for exercise in self.exercises:
            if exercise.raw not in self.exercise_scores:
                self.exercise_scores[exercise.raw] = EXERCISE_DEFAULT_SCORE

        print(f"Exercises: {len(self.exercises)}")

        # Select the first exercise based on weighted probabilities
        self.select_weighted_random_exercise()
        exercise = self.get_exercise()
        # Initialize the board state to the starting position of the exercise
        # (skipping any initial moves that are part of the setup)
        self.moves_done = exercise.skipped_initial_moves
        self.board = exercise.boards[self.moves_done]

    def get_exercise(self) -> Exercise:
        return self.exercises[self.current_exercise_id]

    def select_weighted_random_exercise(self) -> None:
        self.print_exercise_score_distribution()

        # Calculate weights with exponential scaling to emphasize low scores
        weights = {}
        for i, exercise in enumerate(self.exercises):
            if i == self.current_exercise_id:
                continue  # Skip the current exercise
            # Use power function to create stronger bias toward low scores
            weights[i] = (
                1000 / self.exercise_scores[exercise.raw]
            ) ** EXERCISE_WEIGHT_POWER

        if not weights:
            raise NoExercisesLeft()

        # Normalize weights to sum to 1
        total_weight = sum(weights.values())
        normalized_weights = {ex_id: w / total_weight for ex_id, w in weights.items()}

        # Select an exercise based on weights
        exercise_ids = list(normalized_weights.keys())
        weights_list = [normalized_weights[ex_id] for ex_id in exercise_ids]

        self.current_exercise_id = random.choices(
            exercise_ids, weights=weights_list, k=1
        )[0]

    def get_board(self) -> Board:
        return self.board

    def get_ui_details(self) -> dict[str, Any]:
        details: dict[str, Any] = {"move_mistakes": self.move_mistakes}

        try:
            exercise = self.get_exercise()
        except NoExercisesLeft:
            pass
        else:
            if self.moves_done in exercise.forced_move_indices:
                details["forced_move_index"] = exercise.get_forced_move(
                    self.board, self.moves_done
                )

        return details

    def on_move(self, move: int) -> None:
        try:
            exercise = self.get_exercise()
        except NoExercisesLeft:
            return

        board = self.get_board()

        if not board.is_valid_move(move):
            # Invalid move
            return

        normalized_child = board.position.do_normalized_move(move)

        if normalized_child != exercise.get_normalized(self.moves_done + 1):
            # Incorrect move.

            if self.moves_done in exercise.forced_move_indices:
                # Wrong forced move.
                return

            self.move_mistakes.add(move)
            self.exercise_mistakes = True
            return

        self.move_mistakes = set()

        if self.moves_done + 2 >= len(exercise.boards):
            # End of exercise, find next one.
            # Exercise will come back later if mistakes were made.

            # Update score for current exercise based on performance
            self._update_exercise_score(self.exercise_mistakes)

            # Save updated scores to file
            EXERCISE_SCORES_PATH.parent.mkdir(parents=True, exist_ok=True)
            with EXERCISE_SCORES_PATH.open("w") as f:
                json.dump(self.exercise_scores, f, indent=4, sort_keys=True)

            self.select_weighted_random_exercise()
            self.exercise_mistakes = False

            try:
                exercise = self.get_exercise()
            except NoExercisesLeft:
                self.board = Board.empty()
            else:
                self.moves_done = exercise.skipped_initial_moves
                self.board = exercise.boards[self.moves_done]

            return

        self.board = exercise.get_next_board(self.board, move, self.moves_done)
        self.moves_done += 2

    def _update_exercise_score(self, had_mistakes: bool) -> None:
        """Update the score for the current exercise based on performance."""
        current_exercise = self.get_exercise()
        key = current_exercise.raw

        if had_mistakes:
            # Decrease score for incorrect answers (making it more likely to appear)
            self.exercise_scores[key] = max(
                EXERCISE_MIN_SCORE,
                self.exercise_scores[key] - EXERCISE_INCORRECT_DIFF,
            )
        else:
            # Increase score for correct answers (making it less likely to appear)
            self.exercise_scores[key] += EXERCISE_CORRECT_DIFF

    def print_exercise_score_distribution(self) -> None:
        """
        Print a distribution of exercise scores showing:
        - Score value (higher scores = less likely to appear)
        - Probability of selection (%) for exercises with that score
        - Number of exercises with that score
        - Total probability (%) for all exercises with that score

        This helps visualize how the spaced repetition system is prioritizing exercises
        based on past performance.
        """
        # Group exercises by score
        score_counts = {}
        for exercise in self.exercises:
            score = self.exercise_scores[exercise.raw]
            if score not in score_counts:
                score_counts[score] = 0
            score_counts[score] += 1

        # Calculate total inverse weight for probability calculation
        total_inverse_weight = sum(
            (1000 / score) ** EXERCISE_WEIGHT_POWER
            for score in self.exercise_scores.values()
        )

        # Sort scores and print distribution
        print("\nscore       % count  total%")

        for score in sorted(score_counts.keys()):
            count = score_counts[score]

            # Apply the same power function used in selection
            probability = (
                100.0 * ((1000 / score) ** EXERCISE_WEIGHT_POWER) / total_inverse_weight
            )
            print(
                f"{score:>5} {probability:6.2f}% {count:>5} {probability * count:6.2f}%"
            )
