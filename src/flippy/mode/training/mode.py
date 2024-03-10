from __future__ import annotations
from typing import Any
from flippy.mode.training.exercise import Exercise
from flippy.mode.training.loader import ExerciseLoader, ExerciseLoaderArgs
from flippy.mode.training.logs import LogItem
from flippy.othello.board import Board
from flippy.mode.base import BaseMode
import random


class NoExercisesLeft(Exception):
    pass


class TrainingMode(BaseMode):
    def __init__(self) -> None:
        self.exercises: list[Exercise] = []
        self.remaining_exercise_ids: list[int] = []
        self.move_mistakes: set[int] = set()
        self.exercise_mistakes = False

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

    def load_exercises(self, loader_args: ExerciseLoaderArgs) -> None:
        loader = ExerciseLoader(loader_args)
        self.exercises, self.remaining_exercise_ids = loader.get_exercises()
        print(f"Exercises: {len(self.remaining_exercise_ids)}")

        random.shuffle(self.remaining_exercise_ids)

        self.move_mistakes = set()
        self.exercise_mistakes = False

        try:
            exercise = self.get_exercise()
        except NoExercisesLeft:
            return

        self.moves_done = exercise.skipped_initial_moves

    def get_board(self) -> Board:
        try:
            exercise = self.get_exercise()
        except NoExercisesLeft:
            return Board.empty()

        return exercise.boards[self.moves_done]

    def get_ui_details(self) -> dict[str, Any]:
        return {"move_mistakes": self.move_mistakes}

    def on_move(self, move: int) -> None:
        try:
            exercise = self.get_exercise()
        except NoExercisesLeft:
            return

        board = self.get_board()

        if not board.is_valid_move(move):
            # Invalid move
            return

        if move != exercise.moves[self.moves_done]:
            # Incorrect move.
            self.move_mistakes.add(move)
            self.exercise_mistakes = True
            return

        self.moves_done += 2
        self.move_mistakes = set()

        if len(exercise.boards) <= self.moves_done:
            # End of exercise, find next one.
            # Exercise will come back later if mistakes were made.
            LogItem(exercise.get_moves_seq_str(), self.exercise_mistakes).save_to_file()

            self.change_exercise(keep_current=self.exercise_mistakes)
            self.exercise_mistakes = False

            try:
                exercise = self.get_exercise()
            except NoExercisesLeft:
                return

            self.moves_done = exercise.skipped_initial_moves
