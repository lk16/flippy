import requests
import threading
import time
from datetime import datetime
from typing import Optional

from flippy.book.api_client import APIClient
from flippy.book.models import (
    Job,
    JobResult,
    SerializedEvaluation,
)
from flippy.edax.process import evaluate_blocking
from flippy.edax.types import EdaxRequest
from flippy.othello.position import NormalizedPosition


class BookLearningClient:
    def __init__(self) -> None:
        self.api_client = APIClient()
        self.client_id: str | None = None
        self.finished_job_count = 0
        self.total_job_time = 0.0  # seconds

        threading.Thread(target=self._heartbeat_loop, daemon=True).start()

    def _register(self) -> str:
        return self.api_client.register_learn_client()

    def heartbeat(self) -> None:
        if self.client_id is None:
            print("No client ID, skipping heartbeat.")
            return

        self.api_client.heartbeat(self.client_id)

    def get_job(self) -> Optional[Job]:
        assert self.client_id is not None
        return self.api_client.get_learn_job(self.client_id)

    def submit_result(self, result: JobResult) -> None:
        assert self.client_id is not None
        self.api_client.submit_job_result(self.client_id, result)

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
            try:
                if self.client_id is None:
                    print("Getting new client ID")
                    self.client_id = self._register()

                job = self.get_job()
                if job is None:
                    print("No jobs available right now, waiting 10 seconds")
                    time.sleep(10)
                    continue

                position = NormalizedPosition.from_api(job.position).to_position()
                print(
                    f"Got job {self.finished_job_count + 1} | {position.count_discs()} discs | learn level {job.level}:"
                )
                position.show()

                job_result = self.do_job(job)

                self.submit_result(job_result)
                print("Submitted result")

                self.finished_job_count += 1
                self.total_job_time += job_result.computation_time
                avg_job_time = self.total_job_time / self.finished_job_count
                print(
                    f"Total jobs: {self.finished_job_count:>6} | Average time: {avg_job_time:8.3f} sec"
                )
                print()

            except requests.HTTPError as e:
                if e.response.status_code == 401:
                    # Server restarted, re-register in next loop iteration
                    self.client_id = None

                time.sleep(5)  # Back off on error
                print(f"Error: {e}")

            except requests.RequestException as e:
                print(f"Error: {e}")
                time.sleep(5)  # Back off on error

    def do_job(self, job: Job) -> JobResult:
        position = NormalizedPosition.from_api(job.position)

        request = EdaxRequest({position}, job.level, source=None)

        before = datetime.now()
        evaluations = evaluate_blocking(request)
        after = datetime.now()

        computation_time = (after - before).total_seconds()

        return JobResult(
            evaluation=SerializedEvaluation.from_evaluation(evaluations[position]),
            computation_time=computation_time,
        )
