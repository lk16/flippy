from __future__ import annotations
from typing import Optional


class TrainingArguments:
    def __init__(self, filters: list[str], top: Optional[int]) -> None:
        self.filters = filters
        self.top = top


class PositionFrequencyArguments:
    def __init__(self, lost_only: bool, most_recent: Optional[int]) -> None:
        self.lost_only = lost_only
        self.most_recent = most_recent


class Arguments:
    def __init__(
        self,
        training: TrainingArguments,
        position_frequency: PositionFrequencyArguments,
    ) -> None:
        self.training = training
        self.position_frequency = position_frequency

    @classmethod
    def empty(cls) -> Arguments:
        return Arguments(
            TrainingArguments([], None),
            PositionFrequencyArguments(False, None),
        )
