import requests
import subprocess
from pydantic import BaseModel
from requests import Response
from socket import gethostname
from typing import Any, Optional

from flippy.book.models import (
    MAX_BATCH_SIZE,
    EvaluationsPayload,
    Job,
    JobResult,
    LookupPositionsPayload,
    RegisterRequest,
    RegisterResponse,
    SerializedEvaluation,
)
from flippy.config import get_book_server_token, get_book_server_url
from flippy.edax.types import EdaxEvaluation, EdaxEvaluations
from flippy.othello.position import NormalizedPosition


def get_git_commit() -> str:
    try:
        return subprocess.check_output(["git", "rev-parse", "HEAD"]).decode("ascii")[:8]
    except (subprocess.SubprocessError, FileNotFoundError):
        return "unknown"


class APIClient:
    """
    API Client for the book server.

    This client does not keep track of the client ID used for book learning.
    The caller should store it and pass as arguments when necessary.
    """

    def __init__(self) -> None:
        self.server_url = get_book_server_url()
        self.token = get_book_server_token()

    def _get(
        self,
        path: str,
        client_id: Optional[str] = None,
        json: Optional[BaseModel | list[str]] = None,
    ) -> Response:
        headers = {"x-token": self.token}
        if client_id is not None:
            headers["client-id"] = client_id

        kwargs: dict[str, Any] = {"headers": headers}

        if json is not None:
            if isinstance(json, BaseModel):
                kwargs["json"] = json.model_dump()
            else:
                kwargs["json"] = json

        response = requests.get(f"{self.server_url}{path}", **kwargs)

        response.raise_for_status()
        return response

    def _post(
        self,
        path: str,
        client_id: Optional[str] = None,
        json: Optional[BaseModel] = None,
    ) -> Response:
        headers = {"x-token": self.token}
        if client_id is not None:
            headers["client-id"] = client_id

        kwargs: dict[str, Any] = {"headers": headers}

        if json is not None:
            kwargs["json"] = json.model_dump()

        response = requests.post(f"{self.server_url}{path}", **kwargs)
        response.raise_for_status()

        return response

    def register_learn_client(self) -> str:
        payload = RegisterRequest(
            hostname=gethostname(),
            git_commit=get_git_commit(),
        )

        response = self._post("/api/learn-clients/register", json=payload)

        parsed = RegisterResponse.model_validate_json(response.text)
        return parsed.client_id

    def heartbeat(self, client_id: str) -> None:
        response = self._post("/api/learn-clients/heartbeat", client_id=client_id)
        response.raise_for_status()

    def get_learn_job(self, client_id: str) -> Optional[Job]:
        response = self._get("/api/learn-clients/job", client_id=client_id)

        if response.text == "null":
            return None

        return Job.model_validate_json(response.text)

    def submit_job_result(self, client_id: str, payload: JobResult) -> None:
        response = self._post("/api/job/result", client_id=client_id, json=payload)
        response.raise_for_status()

    def lookup_positions(self, positions: set[NormalizedPosition]) -> EdaxEvaluations:
        all_parsed: list[SerializedEvaluation] = []

        positions_list = list(positions)

        for i in range(0, len(positions_list), MAX_BATCH_SIZE):
            chunk = positions_list[i : i + MAX_BATCH_SIZE]
            payload = LookupPositionsPayload(positions=[pos.to_api() for pos in chunk])
            response = self._post("/api/positions/lookup", json=payload)

            all_parsed.extend(
                [SerializedEvaluation.model_validate(item) for item in response.json()]
            )

        evaluations = EdaxEvaluations()

        for item in all_parsed:
            eval = item.to_evaluation()

            # API returns positions in normalized form
            normalized = NormalizedPosition(eval.position)

            evaluations[normalized] = eval

        return evaluations

    def save_learned_evaluations(self, evaluations: list[EdaxEvaluation]) -> None:
        if not evaluations:
            return

        serialized = [
            SerializedEvaluation.from_evaluation(eval) for eval in evaluations
        ]
        payload = EvaluationsPayload(evaluations=serialized)

        self._post("/api/evaluations", json=payload)
