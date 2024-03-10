from __future__ import annotations
import csv
from typing import Optional
from flippy.mode.training.exercise import Exercise
from pathlib import Path

from flippy.mode.training.logs import LogFile


OPENINGS_FILE = Path(__file__).parent / "../../../../openings.csv"


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


class ExerciseLoaderArgs:
    def __init__(self, filters: list[str], top: Optional[int]) -> None:
        self.filters = filters
        self.top = top


class ExerciseLoader:
    def __init__(self, args: ExerciseLoaderArgs) -> None:
        self.str_filters = args.filters
        self.top = args.top
        self.filters: list[BaseFilter] = []
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

    def _load_all_filters(self) -> None:
        csv_columns = self.exercises[0].raw.keys()

        filters: list[BaseFilter] = []
        for str_filter in self.str_filters:
            split = str_filter.split("=")

            if len(split) != 2:
                raise ValueError(f"Failed to handle filter {str_filter}")

            field, value = split

            if field not in csv_columns:
                raise ValueError(f"Cannot filter by {field}")

            filter = self._load_filter(field, value)
            filters.append(filter)

            if field == "moves" and isinstance(filter, PrefixFilter):
                self.skipped_initial_moves = len(filter.prefix.split(" "))

                if self.skipped_initial_moves % 2 == 0:
                    filters.append(ExactFilter("color", "white"))
                else:
                    filters.append(ExactFilter("color", "black"))

        self.filters = filters

    def _load_filter(self, field: str, value: str) -> BaseFilter:
        if value.endswith("..."):
            return PrefixFilter(field, value[:-3].strip())

        return ExactFilter(field, value)

    def get_exercises(self) -> tuple[list[Exercise], list[int]]:
        self._load_all_exercises()
        self._load_all_filters()
        ids: list[int] = []

        for id, exercise in enumerate(self.exercises):
            if exercise.has_skipped_children:
                continue

            if len(exercise.boards) < self.skipped_initial_moves:
                continue

            exercise.skipped_initial_moves = self.skipped_initial_moves

            if exercise.boards[exercise.skipped_initial_moves].turn != exercise.color:
                exercise.skipped_initial_moves += 1

            if all(filter.matches(exercise) for filter in self.filters):
                ids.append(id)

        if self.top is not None:
            ids = self._select_top(ids)

        return self.exercises, ids

    def _select_top(self, ids: list[int]) -> list[int]:
        assert self.top

        exercises = [self.exercises[id] for id in ids]
        top_exercises = LogFile().get_top_n(exercises, self.top)

        top_ids: list[int] = []

        for top_exercise in top_exercises:
            for id, exercise in enumerate(self.exercises):
                if top_exercise == exercise:
                    top_ids.append(id)
                    break

        return top_ids
