from datetime import datetime, timedelta
from fastapi import Depends, FastAPI, Header, HTTPException, Response
from typing import Optional
from uuid import uuid4

from flippy.book import get_learn_level
from flippy.book.models import (
    ClientStats,
    Job,
    JobResponse,
    JobResult,
    RegisterResponse,
    SerializedPosition,
    StatsResponse,
)
from flippy.db import DB, MAX_SAVABLE_DISCS, is_savable_position


class Client:
    def __init__(self, client_id: str):
        self.id = client_id
        self.last_heartbeat = datetime.now()
        self.job: Optional[Job] = None
        self.jobs_completed = 0


class ServerState:
    def __init__(self) -> None:
        self.active_clients: dict[str, Client] = {}
        self.job_queue: list[Job] = []
        self.disc_count = 0
        self.db = DB()
        self.last_new_boards_check_time = datetime.now()
        self.last_prune_time = datetime.now()

    def _load_jobs_for_disc_count(self, disc_count: int) -> list[Job]:
        """Load jobs for positions with the specified disc count that need to be learned."""
        learn_level = get_learn_level(disc_count)
        positions = self.db.get_boards_with_disc_count_below_level(
            disc_count, learn_level
        )

        return [
            Job(
                position=SerializedPosition.from_position(position),
                level=learn_level,
            )
            for position in positions
            if is_savable_position(position)
        ]

    def check_for_new_boards(self) -> None:
        current_time = datetime.now()
        check_interval = timedelta(minutes=10)

        if current_time - self.last_new_boards_check_time < check_interval:
            return

        self.last_new_boards_check_time = current_time
        print("Checking for new boards to learn...")

        # Check all disc counts up to current disc_count
        for disc_count in range(4, self.disc_count + 1):
            new_jobs = self._load_jobs_for_disc_count(disc_count)

            if new_jobs:
                if disc_count == self.disc_count and len(new_jobs) == len(
                    self.job_queue
                ):
                    # No new jobs found
                    continue

                print(f"Found {len(new_jobs)} new positions with {disc_count} discs")
                self.disc_count = disc_count
                self.job_queue = new_jobs
                return

    def get_next_job(self) -> Optional[Job]:
        self.check_for_new_boards()

        while not self.job_queue:
            self.disc_count = max(4, self.disc_count + 1)

            if self.disc_count > MAX_SAVABLE_DISCS:
                return None

            print(f"Loading jobs for positions with {self.disc_count} discs")
            self.job_queue = self._load_jobs_for_disc_count(self.disc_count)

        return self.job_queue.pop()

    def prune_inactive_clients(self) -> None:
        current_time = datetime.now()
        check_interval = timedelta(minutes=1)

        if current_time - self.last_prune_time < check_interval:
            return

        self.last_prune_time = current_time

        inactive_threshold = timedelta(minutes=5)
        min_heartbeat_time = current_time - inactive_threshold

        inactive_client_ids: list[str] = []

        for client_id, client in self.active_clients.items():
            if client.last_heartbeat < min_heartbeat_time:
                inactive_client_ids.append(client_id)

            # Restore job from inactive clients
            if client.job:
                self.job_queue.append(client.job)

        for client_id in inactive_client_ids:
            del self.active_clients[client_id]

        print(f"Pruned {len(inactive_client_ids)} inactive clients")


def get_server_state() -> ServerState:
    """Dependency that provides the server state"""
    return server_state


app = FastAPI()
server_state = ServerState()


@app.post("/register")
async def register_client(
    state: ServerState = Depends(get_server_state),
) -> RegisterResponse:
    client_id = str(uuid4())
    state.active_clients[client_id] = Client(client_id)
    print(f"Registered client {client_id}")
    return RegisterResponse(client_id=client_id)


@app.get("/job")
async def get_job(
    client_id: str = Header(...), state: ServerState = Depends(get_server_state)
) -> JobResponse:
    # We prune clients here, because we don't have to add some timing logic.
    # If nobody asks for work, we don't risk losing any work handed out to inactive clients.
    state.prune_inactive_clients()

    if client_id not in state.active_clients:
        raise HTTPException(status_code=401)

    job = state.get_next_job()
    state.active_clients[client_id].job = job
    return JobResponse(job=job)


@app.post("/heartbeat")
async def heartbeat(
    client_id: str = Header(...), state: ServerState = Depends(get_server_state)
) -> Response:
    if client_id not in state.active_clients:
        raise HTTPException(status_code=401)

    state.active_clients[client_id].last_heartbeat = datetime.now()
    return Response()


@app.post("/job/result")
async def submit_result(
    result: JobResult,
    client_id: str = Header(...),
    state: ServerState = Depends(get_server_state),
) -> Response:
    if client_id not in state.active_clients:
        raise HTTPException(status_code=401)

    state.active_clients[client_id].jobs_completed += 1

    completed = state.active_clients[client_id].jobs_completed
    print(f"Client {client_id} has now completed {completed} positions")

    state.db.save_edax(result.evaluation.to_evaluation())

    return Response()


@app.get("/stats")
async def get_stats(state: ServerState = Depends(get_server_state)) -> StatsResponse:
    clients = state.active_clients.values()

    return StatsResponse(
        active_clients=len(clients),
        client_stats=[
            ClientStats(
                id=client.id,
                positions_computed=client.jobs_completed,
                last_active=client.last_heartbeat,
            )
            for client in clients
        ],
    )
