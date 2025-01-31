from __future__ import annotations

from datetime import datetime
from pydantic import BaseModel

from flippy.edax.types import EdaxEvaluation
from flippy.othello.position import Position


class SerializedPosition(BaseModel):
    me: int
    opp: int

    def to_position(self) -> Position:
        return Position(self.me, self.opp)

    @classmethod
    def from_position(cls, position: Position) -> SerializedPosition:
        return cls(me=position.me, opp=position.opp)


class SerializedEvaluation(BaseModel):
    position: SerializedPosition
    level: int
    depth: int
    confidence: int
    score: int
    best_moves: list[int]

    def to_evaluation(self) -> EdaxEvaluation:
        return EdaxEvaluation(
            position=self.position.to_position(),
            level=self.level,
            depth=self.depth,
            confidence=self.confidence,
            score=self.score,
            best_moves=self.best_moves,
        )

    @classmethod
    def from_evaluation(cls, evaluation: EdaxEvaluation) -> SerializedEvaluation:
        return cls(
            position=SerializedPosition.from_position(evaluation.position),
            level=evaluation.level,
            depth=evaluation.depth,
            confidence=evaluation.confidence,
            score=evaluation.score,
            best_moves=evaluation.best_moves,
        )


class Job(BaseModel):
    position: SerializedPosition
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
    last_active: datetime
