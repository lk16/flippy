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

EXERCISE_CORRECT_DIFF = 20
EXERCISE_INCORRECT_DIFF = 30


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
        # Update score for current exercise based on performance
        current_exercise = self.get_exercise()
        if not self.exercise_mistakes:
            # Increase score for correct answers (making it less likely to appear)
            self.exercise_scores[current_exercise.raw] = (
                self.exercise_scores[current_exercise.raw] + EXERCISE_CORRECT_DIFF
            )

        else:
            # Decrease score for incorrect answers (making it more likely to appear)
            self.exercise_scores[current_exercise.raw] = max(
                EXERCISE_MIN_SCORE,
                self.exercise_scores[current_exercise.raw] - EXERCISE_INCORRECT_DIFF,
            )

        # Save updated scores to file
        EXERCISE_SCORES_PATH.parent.mkdir(parents=True, exist_ok=True)
        with EXERCISE_SCORES_PATH.open("w") as f:
            json.dump(self.exercise_scores, f, indent=4, sort_keys=True)

        # Calculate weights (inverse of scores)
        weights = {}
        for i, exercise in enumerate(self.exercises):
            if i == self.current_exercise_id:
                continue  # Skip the current exercise
            weights[i] = 1000 / self.exercise_scores[exercise.raw]

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
