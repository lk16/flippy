from __future__ import annotations

from datetime import datetime
from pydantic import BaseModel, field_validator
from typing import Optional

from flippy.edax.types import EdaxEvaluation
from flippy.othello.position import NormalizedPosition


class SerializedEvaluation(BaseModel):
    position: str
    level: int
    depth: int
    confidence: int
    score: int
    best_moves: list[int]

    def to_evaluation(self) -> EdaxEvaluation:
        return EdaxEvaluation(
            position=NormalizedPosition.from_api(self.position).to_position(),
            level=self.level,
            depth=self.depth,
            confidence=self.confidence,
            score=self.score,
            best_moves=self.best_moves,
        )

    @classmethod
    def from_evaluation(cls, evaluation: EdaxEvaluation) -> SerializedEvaluation:
        # API returns positions in normalized form
        normalized = NormalizedPosition(evaluation.position)

        return cls(
            position=normalized.to_api(),
            level=evaluation.level,
            depth=evaluation.depth,
            confidence=evaluation.confidence,
            score=evaluation.score,
            best_moves=evaluation.best_moves,
        )


class Job(BaseModel):
    position: str
    level: int


class JobResult(BaseModel):
    evaluation: SerializedEvaluation
    computation_time: float


class RegisterRequest(BaseModel):
    hostname: str
    git_commit: str


class RegisterResponse(BaseModel):
    client_id: str


class StatsResponse(BaseModel):
    active_clients: int
    client_stats: list[ClientStats]


class ClientStats(BaseModel):
    id: str
    hostname: str
    git_commit: str
    positions_computed: int
    last_active: Optional[datetime]


class EvaluationsPayload(BaseModel):
    evaluations: list[SerializedEvaluation]


MAX_BATCH_SIZE = 1000


class LookupPositionsPayload(BaseModel):
    positions: list[str]

    @field_validator("positions")
    @classmethod
    def validate_positions_length(cls, v: list[str]) -> list[str]:
        if len(v) > MAX_BATCH_SIZE:
            raise ValueError(
                f"Cannot request more than {MAX_BATCH_SIZE} positions at once"
            )
        return v
