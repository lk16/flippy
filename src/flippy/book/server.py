import secrets
from datetime import datetime, timedelta
from fastapi import Depends, FastAPI, Header, HTTPException, Response, status
from fastapi.responses import HTMLResponse
from fastapi.security import HTTPBasic, HTTPBasicCredentials
from fastapi.staticfiles import StaticFiles
from pathlib import Path
from typing import Optional
from uuid import uuid4

from flippy.book import get_learn_level
from flippy.book.models import (
    ClientStats,
    Job,
    JobResponse,
    JobResult,
    RegisterRequest,
    RegisterResponse,
    SerializedPosition,
    StatsResponse,
)
from flippy.config import BookServerConfig, get_book_server_token
from flippy.db import DB, MAX_SAVABLE_DISCS, is_savable_position


class Client:
    def __init__(self, client_id: str, hostname: str, git_commit: str):
        self.id = client_id
        self.hostname = hostname
        self.git_commit = git_commit
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
        self.token = get_book_server_token()

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

static_dir = Path(__file__).parent / "static"
app.mount("/static", StaticFiles(directory=static_dir), name="static")


def verify_credentials(
    credentials: HTTPBasicCredentials = Depends(HTTPBasic()),
) -> None:
    config = BookServerConfig()
    correct_username = config.basic_auth_user
    correct_password = config.basic_auth_pass

    is_correct_username = secrets.compare_digest(
        credentials.username.encode("utf8"), correct_username.encode("utf8")
    )
    is_correct_password = secrets.compare_digest(
        credentials.password.encode("utf8"), correct_password.encode("utf8")
    )

    if not (is_correct_username and is_correct_password):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Incorrect username or password",
            headers={"WWW-Authenticate": "Basic"},
        )


@app.post("/register")
async def register_client(
    payload: RegisterRequest,
    state: ServerState = Depends(get_server_state),
    x_token: str = Header(...),
) -> RegisterResponse:
    if x_token != state.token:
        raise HTTPException(status_code=403)

    client_id = str(uuid4())
    state.active_clients[client_id] = Client(
        client_id, payload.hostname, payload.git_commit
    )
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
async def get_stats(
    credentials: HTTPBasicCredentials = Depends(verify_credentials),
    state: ServerState = Depends(get_server_state),
) -> StatsResponse:
    clients = state.active_clients.values()

    return StatsResponse(
        active_clients=len(clients),
        client_stats=[
            ClientStats(
                id=client.id,
                hostname=client.hostname,
                git_commit=client.git_commit,
                positions_computed=client.jobs_completed,
                last_active=client.last_heartbeat,
            )
            for client in clients
        ],
    )


@app.get("/stats/book")
async def get_book_stats(
    state: ServerState = Depends(get_server_state),
) -> list[list[str]]:
    stats = state.db._get_edax_stats()

    disc_counts = sorted(set(row[0] for row in stats))
    levels = sorted(set(row[1] for row in stats))

    lookup: dict[tuple[int, int], int] = {}
    level_totals: dict[int, int] = {}
    disc_totals: dict[int, int] = {}

    for discs, level, count in stats:
        lookup[(discs, level)] = count

        if level not in level_totals:
            level_totals[level] = 0
        level_totals[level] += count

        if discs not in disc_totals:
            disc_totals[discs] = 0
        disc_totals[discs] += count

    table = [[""] + [f"level {level}" for level in levels] + ["Total"]]

    for discs in disc_counts:
        row = [f"{discs} discs"]
        for level in levels:
            row.append(str(lookup.get((discs, level), 0)))
        row.append(str(disc_totals.get(discs, 0)))
        table.append(row)

    table.append(
        ["Total"]
        + [str(level_totals.get(level, 0)) for level in levels]
        + [str(sum(level_totals.values()))]
    )
    return table


@app.get("/book", response_class=HTMLResponse)
async def show_book_stats(
    credentials: HTTPBasicCredentials = Depends(verify_credentials),
) -> str:
    return (static_dir / "book.html").read_text()


@app.get("/", response_class=HTMLResponse)
async def show_clients(
    credentials: HTTPBasicCredentials = Depends(verify_credentials),
) -> str:
    return (static_dir / "clients.html").read_text()
