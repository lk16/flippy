from __future__ import annotations
from collections import defaultdict
from datetime import datetime
import json

from pathlib import Path
from typing import DefaultDict, Optional

from flippy.mode.training.exercise import Exercise
from flippy.othello.board import Board


LOG_FILE = Path(__file__).parent / "../../../../opening_logs.txt"


class LogItem:
    def __init__(
        self, exercise_id: str, mistakes: bool, timestamp: Optional[datetime] = None
    ):
        self.exercise_id = exercise_id
        self.mistakes = mistakes
        self.timestamp = timestamp or datetime.now()

    @classmethod
    def from_json(cls, raw: str) -> LogItem:
        parsed = json.loads(raw)
        exercise_id = parsed["exercise"]
        mistakes = parsed["mistakes"]
        timestamp = datetime.fromisoformat(parsed["timestamp"])
        return LogItem(exercise_id, mistakes, timestamp)

    def to_json(self) -> str:
        return json.dumps(
            {
                "exercise": self.exercise_id,
                "mistakes": self.mistakes,
                "timestamp": self.timestamp.isoformat(),
            }
        )

    def save_to_file(self) -> None:
        with LOG_FILE.open("a") as file:
            file.write(self.to_json() + "\n")


class LogFile:
    def __init__(self) -> None:
        self.log_items: list[LogItem] = []

        for line in LOG_FILE.read_text().split("\n"):
            if not line.strip():
                continue
            self.log_items.append(LogItem.from_json(line))

        self.attempt_count: DefaultDict[str, int] = defaultdict(lambda: 0)
        self.correct_count: DefaultDict[str, int] = defaultdict(lambda: 0)
        self.mistake_count: DefaultDict[str, int] = defaultdict(lambda: 0)
        self.last_correct: DefaultDict[str, datetime] = defaultdict(
            lambda: datetime.min
        )
        self.last_mistake: DefaultDict[str, datetime] = defaultdict(
            lambda: datetime.min
        )

        for log_item in self.log_items:
            id = log_item.exercise_id

            self.attempt_count[id] += 1

            if log_item.mistakes:
                self.mistake_count[id] += 1
                self.last_mistake[id] = min(self.last_mistake[id], log_item.timestamp)
            else:
                self.correct_count[id] += 1
                self.last_correct[id] = min(self.last_correct[id], log_item.timestamp)

    def get_priority(self, exercise: Exercise) -> int:
        assert not exercise.has_skipped_children
        assert exercise.eval is not None

        id = exercise.get_moves_seq_str()
        attempts = self.attempt_count[id]
        correct = self.correct_count[id]
        mistake = self.mistake_count[id]
        last_correct = self.last_correct[id]
        last_mistake = self.last_mistake[id]
        eval = exercise.eval
        interest = exercise.raw["interest"]
        move_count = len(exercise.moves)

        priority = 0

        if attempts == 0:
            priority += 100
        else:
            if attempts <= 3:
                priority += 100 // attempts
            else:
                priority += int(33 * (mistake / attempts))

        priority += 5 * mistake
        priority -= 3 * correct

        if last_correct > last_mistake:
            priority -= 10

        priority -= eval

        priority += 5 * move_count

        priority += {
            "tp": -20,
            "vlow": -50,
            "low": -20,
            "mid": 0,
            "high": 20,
            "vhigh": 40,
        }[interest]

        return priority

    def get_top_n(self, exercises: list[Exercise], n: int) -> list[Exercise]:
        def sort_key(t: tuple[Exercise, int]) -> int:
            return t[1]

        priorities = [(exercise, self.get_priority(exercise)) for exercise in exercises]

        priorities.sort(key=sort_key, reverse=True)

        print("priority | correct | attempts | interest | eval | moves")

        for exercise, priority in priorities[:n]:
            id = exercise.get_moves_seq_str()
            attempts = self.attempt_count[id]
            correct = self.correct_count[id]
            eval = exercise.eval
            interest = exercise.raw["interest"]

            correct_percentage = 0.0
            if attempts != 0:
                correct_percentage = 100.0 * (correct / attempts)

            print(
                f"{priority:>8} | {correct_percentage:6.2f}% | {attempts:>8} | {interest:>8} | {eval:>4} | {Board.offsets_to_str(exercise.moves)}"
            )

        return [item[0] for item in priorities][:n]
