import requests
import socket
import subprocess
import threading
import time
from datetime import datetime
from typing import Optional

from flippy.book.models import (
    Job,
    JobResponse,
    JobResult,
    RegisterRequest,
    RegisterResponse,
    SerializedEvaluation,
)
from flippy.config import get_book_server_token, get_book_server_url
from flippy.edax.process import start_evaluation_sync
from flippy.edax.types import EdaxRequest


class BookLearningClient:
    def __init__(self) -> None:
        self.server_url = get_book_server_url()
        self.client_id: str | None = None
        self.hostname = socket.gethostname()
        self.token = get_book_server_token()
        try:
            self.git_commit = subprocess.check_output(
                ["git", "rev-parse", "HEAD"]
            ).decode("ascii")[:8]
        except (subprocess.SubprocessError, FileNotFoundError):
            self.git_commit = "unknown"

        threading.Thread(target=self._heartbeat_loop, daemon=True).start()

    def _register(self) -> str:
        response = requests.post(
            f"{self.server_url}/register",
            json=RegisterRequest(
                hostname=self.hostname,
                git_commit=self.git_commit,
            ).model_dump(),
            headers={"x-token": self.token},
        )
        response.raise_for_status()

        parsed = RegisterResponse.model_validate_json(response.text)
        return parsed.client_id

    def heartbeat(self) -> None:
        if self.client_id is None:
            return

        response = requests.post(
            f"{self.server_url}/heartbeat", headers={"client-id": self.client_id}
        )
        response.raise_for_status()

    def get_job(self) -> Optional[Job]:
        assert self.client_id is not None

        response = requests.get(
            f"{self.server_url}/job", headers={"client-id": self.client_id}
        )
        response.raise_for_status()
        parsed = JobResponse.model_validate_json(response.text)
        return parsed.job

    def submit_result(self, result: JobResult) -> None:
        assert self.client_id is not None

        payload = result.model_dump()
        response = requests.post(
            f"{self.server_url}/job/result",
            headers={"client-id": self.client_id},
            json=payload,
        )
        response.raise_for_status()

    def _heartbeat_loop(self) -> None:
        while True:
            try:
                self.heartbeat()
            except Exception as e:
                print(f"Heartbeat error: {e}")
                # We don't handle HTTP 401 here, we only do that in the main thread.
            time.sleep(60)  # Sleep for 1 minute

    def run(self) -> None:
        while True:
            if self.client_id is None:
                print("Getting new client ID")
                self.client_id = self._register()
                self.headers = {"client-id": self.client_id}

            try:
                job = self.get_job()
                if job is None:
                    # No more positions to compute
                    break

                print("Got a job")
                job_result = self.do_job(job)

                self.submit_result(job_result)
                print("Submitted result")

            except requests.HTTPError as e:
                if e.response.status_code == 401:
                    # Server restarted, re-register in next loop iteration
                    self.client_id = None
                else:
                    raise e

            except Exception as e:
                print(f"Error: {e}")
                time.sleep(5)  # Back off on error

    def do_job(self, job: Job) -> JobResult:
        position = job.position.to_position()

        request = EdaxRequest([position], job.level, source=None)

        before = datetime.now()
        evaluations = start_evaluation_sync(request)
        after = datetime.now()

        computation_time = (after - before).total_seconds()

        return JobResult(
            evaluation=SerializedEvaluation.from_evaluation(
                evaluations.lookup(position)
            ),
            computation_time=computation_time,
        )
