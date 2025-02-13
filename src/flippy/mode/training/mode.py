from __future__ import annotations

import random
from typing import Any

from flippy.arguments import Arguments
from flippy.mode.base import BaseMode
from flippy.mode.training.exercise import Exercise
from flippy.mode.training.exercise_list import get_exercises
from flippy.othello.board import Board


class NoExercisesLeft(Exception):
    pass


class TrainingMode(BaseMode):
    def __init__(self, _: Arguments) -> None:
        self.exercises = get_exercises()
        self.remaining_exercise_ids = list(range(len(self.exercises)))
        self.move_mistakes: set[int] = set()
        self.exercise_mistakes = False

        print(f"Exercises: {len(self.remaining_exercise_ids)}")
        random.shuffle(self.remaining_exercise_ids)

        try:
            exercise = self.get_exercise()
        except NoExercisesLeft:
            self.moves_done = 0
            self.board = Board.empty()
        else:
            self.moves_done = exercise.skipped_initial_moves
            self.board = exercise.boards[self.moves_done]

    def get_exercise(self) -> Exercise:
        try:
            exercise_id = self.remaining_exercise_ids[0]
        except IndexError as e:
            raise NoExercisesLeft from e

        return self.exercises[exercise_id]

    def change_exercise(self, keep_current: bool) -> None:
        try:
            current_exercise_id = self.remaining_exercise_ids.pop(0)
        except IndexError as e:
            raise NoExercisesLeft from e

        if keep_current:
            self.remaining_exercise_ids.append(current_exercise_id)

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

            self.change_exercise(keep_current=self.exercise_mistakes)
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
