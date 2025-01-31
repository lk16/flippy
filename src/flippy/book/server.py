import asyncpg  # type:ignore[import-untyped]
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
    JobResult,
    RegisterRequest,
    RegisterResponse,
    SerializedPosition,
    StatsResponse,
)
from flippy.config import BookServerConfig, get_book_server_token, get_db_dsn
from flippy.db import MAX_SAVABLE_DISCS, is_savable_evaluation
from flippy.othello.position import Position


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
        # TODO move active clients into DB and remove this
        self.active_clients: dict[str, Client] = {}

        self.token = get_book_server_token()

        # Initialize as None, will connect lazily
        self.db: asyncpg.Connection | None = None

    async def get_db(self) -> asyncpg.Connection:
        if self.db is None:
            self.db = await asyncpg.connect(get_db_dsn())
        return self.db

    def prune_inactive_clients(self) -> None:
        inactive_threshold = timedelta(minutes=5)
        min_heartbeat_time = datetime.now() - inactive_threshold

        inactive_client_ids: list[str] = []

        for client_id, client in self.active_clients.items():
            if client.last_heartbeat < min_heartbeat_time:
                inactive_client_ids.append(client_id)

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


def verify_token(
    x_token: str = Header(...),
    state: ServerState = Depends(get_server_state),
) -> None:
    if x_token != state.token:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Invalid token",
        )


# TODO unify auth, we have verify_credentials, verify_token and BasicAuth


@app.post("/api/register")
async def register_client(
    payload: RegisterRequest,
    state: ServerState = Depends(get_server_state),
    _: None = Depends(verify_token),
) -> RegisterResponse:
    client_id = str(uuid4())
    state.active_clients[client_id] = Client(
        client_id, payload.hostname, payload.git_commit
    )
    print(f"Registered client {client_id}")
    return RegisterResponse(client_id=client_id)


@app.post("/api/heartbeat")
async def heartbeat(
    client_id: str = Header(...), state: ServerState = Depends(get_server_state)
) -> Response:
    if client_id not in state.active_clients:
        raise HTTPException(status_code=401)

    state.active_clients[client_id].last_heartbeat = datetime.now()
    return Response()


@app.post("/api/job/result")
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

    evaluation = result.evaluation.to_evaluation()
    if not is_savable_evaluation(evaluation):
        return Response(status_code=400, content="Evaluation is not savable")

    conn = await state.get_db()

    evaluation._validate()
    assert evaluation.position.is_normalized()

    disc_count = evaluation.position.count_discs()

    # TODO remove column learn_priority from DB
    priority = 3 * evaluation.level + disc_count

    query = """
    INSERT INTO edax (position, disc_count, level, depth, confidence, score, learn_priority, best_moves)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    ON CONFLICT (position)
    DO UPDATE SET
        level = EXCLUDED.level,
        depth = EXCLUDED.depth,
        confidence = EXCLUDED.confidence,
        score = EXCLUDED.score,
        learn_priority = EXCLUDED.learn_priority,
        best_moves = EXCLUDED.best_moves
    WHERE edax.level < EXCLUDED.level
    """

    await conn.execute(
        query,
        evaluation.position.to_bytes(),
        evaluation.position.count_discs(),
        evaluation.level,
        evaluation.depth,
        evaluation.confidence,
        evaluation.score,
        priority,
        evaluation.best_moves,
    )

    return Response()


@app.get("/api/stats/clients")
async def get_stats(
    _: None = Depends(verify_credentials),
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


@app.get("/api/stats/book")
async def get_book_stats(
    state: ServerState = Depends(get_server_state),
) -> list[list[str]]:
    query = """
    SELECT disc_count, level, COUNT(*)
    FROM edax
    GROUP BY disc_count, level;
    """

    conn = await state.get_db()
    stats = await conn.fetch(query)

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


async def get_job_for_disc_count(
    conn: asyncpg.Connection, disc_count: int
) -> Optional[Job]:
    query = """
        SELECT position
        FROM edax
        WHERE disc_count = $1
        AND level < $2
        ORDER BY level
        LIMIT 1
    """

    learn_level = get_learn_level(disc_count)
    row = await conn.fetchrow(query, disc_count, learn_level)

    if not row:
        return None

    position = SerializedPosition.from_position(Position.from_bytes(row["position"]))
    return Job(position=position, level=learn_level)


@app.get("/api/job")
async def get_job(
    state: ServerState = Depends(get_server_state), client_id: str = Header(...)
) -> Optional[Job]:
    if client_id not in state.active_clients:
        raise HTTPException(status_code=401)

    # We prune clients here, because we don't have to add some timing logic.
    # If nobody asks for work, we don't risk losing any work handed out to inactive clients.
    state.prune_inactive_clients()

    conn = await state.get_db()
    # TODO this is inefficient, once we keep track of items in `edax` table per disc count and level, we can just query the first item
    for disc_count in range(4, MAX_SAVABLE_DISCS + 1):
        job = await get_job_for_disc_count(conn, disc_count)
        if job:
            return job

    return None


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
