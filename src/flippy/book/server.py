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

from flippy.book import MAX_SAVABLE_DISCS, get_learn_level
from flippy.book.models import (
    ClientStats,
    EvaluationsPayload,
    Job,
    JobResult,
    LookupPositionsPayload,
    RegisterRequest,
    RegisterResponse,
    SerializedEvaluation,
    StatsResponse,
)
from flippy.config import BookServerConfig, get_book_server_token, get_db_dsn
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

    async def get_db(self) -> asyncpg.Connection:
        return await asyncpg.connect(get_db_dsn())

    def prune_inactive_clients(self) -> None:
        inactive_threshold = timedelta(minutes=5)
        min_heartbeat_time = datetime.now() - inactive_threshold

        inactive_client_ids: list[str] = []

        for client_id, client in self.active_clients.items():
            if client.last_heartbeat < min_heartbeat_time:
                inactive_client_ids.append(client_id)

        for client_id in inactive_client_ids:
            del self.active_clients[client_id]

        if len(inactive_client_ids) > 0:
            print(f"Pruned {len(inactive_client_ids)} inactive clients")


def get_server_state() -> ServerState:
    """Dependency that provides the server state"""
    return server_state


app = FastAPI()
server_state = ServerState()

static_dir = Path(__file__).parent / "static"
app.mount("/static", StaticFiles(directory=static_dir), name="static")


def verify_auth(
    credentials: Optional[HTTPBasicCredentials] = Depends(HTTPBasic(auto_error=False)),
    x_token: Optional[str] = Header(None),
    state: ServerState = Depends(get_server_state),
) -> None:
    # Try token auth first
    if x_token is not None:
        if x_token == state.token:
            return

    # Fall back to basic auth
    if credentials is not None:
        config = BookServerConfig()
        is_correct_username = secrets.compare_digest(
            credentials.username.encode("utf8"), config.basic_auth_user.encode("utf8")
        )
        is_correct_password = secrets.compare_digest(
            credentials.password.encode("utf8"), config.basic_auth_pass.encode("utf8")
        )
        if is_correct_username and is_correct_password:
            return

    raise HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Invalid authentication",
        headers={"WWW-Authenticate": "Basic"},
    )


@app.post("/api/register")
async def register_client(
    payload: RegisterRequest,
    state: ServerState = Depends(get_server_state),
    _: None = Depends(verify_auth),
) -> RegisterResponse:
    client_id = str(uuid4())
    state.active_clients[client_id] = Client(
        client_id, payload.hostname, payload.git_commit
    )
    print(f"Registered client {client_id}")
    return RegisterResponse(client_id=client_id)


@app.post("/api/heartbeat")
async def heartbeat(
    client_id: str = Header(...),
    _: None = Depends(verify_auth),
    state: ServerState = Depends(get_server_state),
) -> Response:
    if client_id not in state.active_clients:
        raise HTTPException(status_code=401)

    state.active_clients[client_id].last_heartbeat = datetime.now()
    return Response()


@app.post("/api/evaluations")
async def submit_evaluations(
    payload: EvaluationsPayload,
    _: None = Depends(verify_auth),
    state: ServerState = Depends(get_server_state),
) -> Response:
    for evaluation in payload.evaluations:
        try:
            await upsert_evaluation(evaluation, state)
        except ValueError as e:
            raise HTTPException(status_code=400, detail=str(e))

    return Response()


@app.post("/api/job/result")
async def submit_result(
    result: JobResult,
    client_id: str = Header(...),
    _: None = Depends(verify_auth),
    state: ServerState = Depends(get_server_state),
) -> Response:
    if client_id not in state.active_clients:
        raise HTTPException(status_code=401)

    state.active_clients[client_id].jobs_completed += 1

    completed = state.active_clients[client_id].jobs_completed
    print(f"Client {client_id} has now completed {completed} positions")

    try:
        await upsert_evaluation(result.evaluation, state)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    return Response()


async def upsert_evaluation(
    serialized_evaluation: SerializedEvaluation, state: ServerState
) -> None:
    evaluation = serialized_evaluation.to_evaluation()

    if not evaluation.is_db_savable():
        raise ValueError("Evaluation is not savable")

    try:
        evaluation.validate()
    except ValueError as e:
        raise ValueError(f"Evaluation is not valid: {e}") from e

    if not evaluation.position.is_normalized():
        raise ValueError("Position is not normalized")

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

    conn = await state.get_db()

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


@app.get("/api/stats/clients")
async def get_stats(
    _: None = Depends(verify_auth),
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
    _: None = Depends(verify_auth),
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

    position = Position.from_bytes(row["position"])
    return Job(position=position.to_api(), level=learn_level)


@app.get("/api/job")
async def get_job(
    state: ServerState = Depends(get_server_state),
    _: None = Depends(verify_auth),
    client_id: str = Header(...),
) -> Optional[Job]:
    if client_id not in state.active_clients:
        raise HTTPException(status_code=401)

    # We prune clients here, because otherwise we have to call this periodically and setup timing logic.
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
    _: None = Depends(verify_auth),
) -> str:
    return (static_dir / "book.html").read_text()


@app.get("/", response_class=HTMLResponse)
async def show_clients(
    _: None = Depends(verify_auth),
) -> str:
    return (static_dir / "clients.html").read_text()


MAX_POSITION_LOOKUP_SIZE = 1000


@app.post("/api/positions/lookup")
async def get_positions(
    payload: LookupPositionsPayload,
    _: None = Depends(verify_auth),
    state: ServerState = Depends(get_server_state),
) -> list[SerializedEvaluation]:
    positions = [Position.from_api(pos) for pos in payload.positions]

    if len(positions) > MAX_POSITION_LOOKUP_SIZE:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Cannot request more than {MAX_POSITION_LOOKUP_SIZE} positions at once",
        )

    for position in positions:
        if not position.is_normalized():
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail="Positions must be normalized",
            )

    conn = await state.get_db()
    position_bytes = [pos.to_bytes() for pos in positions]

    query = """
    SELECT position, level, depth, confidence, score, best_moves
    FROM edax
    WHERE position = ANY($1)
    """

    rows = await conn.fetch(query, position_bytes)

    results = []
    for row in rows:
        position = Position.from_bytes(row["position"])
        results.append(
            SerializedEvaluation(
                position=position.to_api(),
                level=row["level"],
                depth=row["depth"],
                confidence=row["confidence"],
                score=row["score"],
                best_moves=row["best_moves"],
            )
        )

    return results
