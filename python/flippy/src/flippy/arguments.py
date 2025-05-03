from __future__ import annotations

from pathlib import Path
from typing import Optional


class PositionFrequencyArguments:
    def __init__(self, lost_only: bool, most_recent: Optional[int]) -> None:
        self.lost_only = lost_only
        self.most_recent = most_recent


class PGNArguments:
    def __init__(self, pgn_file: Optional[Path]) -> None:
        self.pgn_file = pgn_file


class Arguments:
    def __init__(
        self,
        position_frequency: PositionFrequencyArguments,
        pgn: PGNArguments,
    ) -> None:
        self.position_frequency = position_frequency
        self.pgn = pgn

    @classmethod
    def empty(cls) -> Arguments:
        return Arguments(
            PositionFrequencyArguments(False, None),
            PGNArguments(None),
        )
