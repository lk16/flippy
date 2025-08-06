from __future__ import annotations

import csv

from flippy import PROJECT_ROOT
from flippy.mode.training.exercise import Exercise

OPENINGS_FILE = PROJECT_ROOT / "openings.csv"


class BaseFilter:
    def matches(self, exercise: Exercise) -> bool:
        raise NotImplementedError


class ExactFilter(BaseFilter):
    def __init__(self, field: str, value: str) -> None:
        self.field = field
        self.values = value.split(",")

    def matches(self, exercise: Exercise) -> bool:
        return exercise.raw[self.field] in self.values


class PrefixFilter(BaseFilter):
    def __init__(self, field: str, prefix: str) -> None:
        self.field = field
        self.prefix = prefix

    def matches(self, exercise: Exercise) -> bool:
        return exercise.raw[self.field].startswith(self.prefix)


class ExerciseLoader:
    def __init__(self) -> None:
        self.exercises: list[Exercise] = []
        self.skipped_initial_moves = 0

    def _load_all_exercises(self) -> None:
        reader = csv.DictReader(open(OPENINGS_FILE, "r"), delimiter="|")
        exercises: list[Exercise] = []
        for row in reader:
            stripped_row = {k.strip(): v.strip() for (k, v) in row.items()}
            exercise = Exercise(stripped_row)
            exercises.append(exercise)

        self.exercises = exercises

    def _load_filter(self, field: str, value: str) -> BaseFilter:
        if value.endswith("..."):
            return PrefixFilter(field, value[:-3].strip())

        return ExactFilter(field, value)

    def get_exercises(self) -> tuple[list[Exercise], list[int]]:
        self._load_all_exercises()
        ids: list[int] = []

        for id, exercise in enumerate(self.exercises):
            if exercise.has_skipped_children:
                continue

            if len(exercise.boards) < self.skipped_initial_moves:
                continue

            exercise.skipped_initial_moves = self.skipped_initial_moves

            if exercise.boards[exercise.skipped_initial_moves].turn != exercise.color:
                exercise.skipped_initial_moves += 1

            ids.append(id)

        return self.exercises, ids
