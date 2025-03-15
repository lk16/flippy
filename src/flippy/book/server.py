import asyncpg  # type:ignore[import-untyped]
import secrets
from fastapi import (
    BackgroundTasks,
    Depends,
    FastAPI,
    Header,
    HTTPException,
    Response,
    status,
)
from fastapi.responses import HTMLResponse
from fastapi.security import HTTPBasic, HTTPBasicCredentials
from fastapi.staticfiles import StaticFiles
from pathlib import Path
from typing import Optional
from uuid import UUID, uuid4

from flippy.book import get_learn_level
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
from flippy.othello.position import NormalizedPosition


class ServerState:
    def __init__(self) -> None:
        self.token = get_book_server_token()
        # Create event loop to run async init
        import asyncio

        asyncio.create_task(self._init_db())

    async def _init_db(self) -> None:
        """Initialize database state on server start"""
        conn = await self.get_db()
        await conn.execute("TRUNCATE TABLE clients")
        print("Truncated clients table on server start")

    async def get_db(self) -> asyncpg.Connection:
        return await asyncpg.connect(get_db_dsn())

    async def prune_inactive_clients(self) -> None:
        conn = await self.get_db()
        deleted: list[dict[str, UUID]] = await conn.fetch(
            """
            DELETE FROM clients
            WHERE last_heartbeat < CURRENT_TIMESTAMP - INTERVAL '5 minutes'
            OR last_heartbeat IS NULL
            RETURNING id
            """,
        )

        if deleted:
            print(f"Pruned {len(deleted)} inactive clients from database")


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


@app.post("/api/learn-clients/register")
async def register_client(
    payload: RegisterRequest,
    state: ServerState = Depends(get_server_state),
    _: None = Depends(verify_auth),
) -> RegisterResponse:
    client_id = str(uuid4())

    conn = await state.get_db()
    async with conn.transaction():
        await conn.execute(
            """
            INSERT INTO clients (id, hostname, git_commit, position, last_heartbeat)
            VALUES ($1, $2, $3, NULL, NOW())
            """,
            client_id,
            payload.hostname,
            payload.git_commit,
        )

    print(f"Registered client {client_id}")
    return RegisterResponse(client_id=client_id)


async def validate_client_id(
    client_id: str = Header(...),
    state: ServerState = Depends(get_server_state),
) -> str:
    """Validates that a client ID exists in the database and returns it."""
    conn = await state.get_db()
    exists = await conn.fetchval(
        "SELECT EXISTS(SELECT 1 FROM clients WHERE id = $1)",
        client_id,
    )
    if not exists:
        raise HTTPException(status_code=401, detail="Invalid client ID")
    return client_id


@app.post("/api/learn-clients/heartbeat")
async def heartbeat(
    client_id: str = Depends(validate_client_id),
    state: ServerState = Depends(get_server_state),
) -> Response:
    conn = await state.get_db()
    await conn.execute(
        """
        UPDATE clients
        SET last_heartbeat = NOW()
        WHERE id = $1
        """,
        UUID(client_id),
    )
    return Response()


@app.get("/api/learn-clients")
async def get_learn_clients(
    _: None = Depends(verify_auth),
    state: ServerState = Depends(get_server_state),
) -> StatsResponse:
    conn = await state.get_db()
    rows = await conn.fetch("SELECT * FROM clients ORDER BY jobs_completed DESC")

    clients = [
        ClientStats(
            id=str(row["id"]),
            hostname=row["hostname"],
            git_commit=row["git_commit"],
            positions_computed=row["jobs_completed"],
            last_active=row["last_heartbeat"],
        )
        for row in rows
    ]

    return StatsResponse(active_clients=len(clients), client_stats=clients)


@app.post("/api/evaluations")
async def submit_evaluations(
    payload: EvaluationsPayload,
    background_tasks: BackgroundTasks,
    _: None = Depends(verify_auth),
    state: ServerState = Depends(get_server_state),
) -> Response:
    for evaluation in payload.evaluations:
        try:
            await upsert_evaluation(evaluation, state)
        except ValueError as e:
            raise HTTPException(status_code=400, detail=str(e))

    background_tasks.add_task(refresh_stats_view, state)
    return Response()


async def refresh_stats_view(state: ServerState) -> None:
    conn = await state.get_db()
    await conn.execute("REFRESH MATERIALIZED VIEW edax_stats_view")


@app.post("/api/job/result")
async def submit_result(
    result: JobResult,
    background_tasks: BackgroundTasks,
    client_id: str = Depends(validate_client_id),
    state: ServerState = Depends(get_server_state),
) -> Response:
    # Update client state
    conn = await state.get_db()

    completed: int = await conn.fetchval(
        """
        UPDATE clients
        SET jobs_completed = jobs_completed + 1, position = NULL
        WHERE id = $1
        RETURNING jobs_completed
        """,
        client_id,
    )

    print(f"Client {client_id} has now completed {completed} positions")

    try:
        await upsert_evaluation(result.evaluation, state)
        background_tasks.add_task(refresh_stats_view, state)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    return Response()


async def upsert_evaluation(
    serialized_evaluation: SerializedEvaluation, state: ServerState
) -> None:
    try:
        evaluation = serialized_evaluation.to_evaluation()
    except ValueError as e:
        raise ValueError(f"Evaluation is not valid: {e}") from e

    if not evaluation.is_db_savable():
        raise ValueError("Evaluation is not savable")

    try:
        normalized = NormalizedPosition(evaluation.position)
    except ValueError as e:
        raise ValueError("Position is not normalized") from e

    conn = await state.get_db()

    await conn.execute(
        """
        INSERT INTO edax (position, disc_count, level, depth, confidence, score, best_moves)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        ON CONFLICT (position)
        DO UPDATE SET
            level = EXCLUDED.level,
            depth = EXCLUDED.depth,
            confidence = EXCLUDED.confidence,
            score = EXCLUDED.score,
            best_moves = EXCLUDED.best_moves
        WHERE EXCLUDED.level > edax.level
        """,
        normalized.to_bytes(),
        evaluation.position.count_discs(),
        evaluation.level,
        evaluation.depth,
        evaluation.confidence,
        evaluation.score,
        evaluation.best_moves,
    )


@app.get("/api/stats/book")
async def get_book_stats(
    state: ServerState = Depends(get_server_state),
    _: None = Depends(verify_auth),
) -> list[list[str]]:
    conn = await state.get_db()
    stats = await conn.fetch("SELECT disc_count, level, count FROM edax_stats_view")

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


@app.get("/api/job")
async def get_job(
    state: ServerState = Depends(get_server_state),
    _: None = Depends(verify_auth),
    client_id: str = Depends(validate_client_id),
) -> Optional[Job]:
    # We prune clients here, because otherwise we have to call this periodically and setup timing logic.
    # If nobody asks for work, we don't risk losing any work handed out to inactive clients.
    await state.prune_inactive_clients()

    conn = await state.get_db()

    async with conn.transaction():
        # Find disc counts that have positions needing work using edax_stats_view
        query = """
            SELECT disc_count, level
            FROM edax_stats_view
            WHERE count > 0
            ORDER BY disc_count, level
        """
        rows = await conn.fetch(query)

        # We could do this in SQL, but we want to keep the get_learn_level logic in one place.
        learnable_disc_counts = sorted(
            {
                row["disc_count"]
                for row in rows
                if row["level"] < get_learn_level(row["disc_count"])
            }
        )

        normalized: Optional[NormalizedPosition] = None

        # Try to find job at different disc counts, starting with the lowest.
        # It is rare that we will have to try more than one disc count.
        for learnable_disc_count in learnable_disc_counts:
            # Get a random position for this disc count that isn't currently assigned
            # The level is not relevant as long as it's below the learn level.
            learn_level = get_learn_level(learnable_disc_count)

            query = """
                SELECT position
                FROM edax
                WHERE disc_count = $1
                AND level < $2
                AND position NOT IN (
                    SELECT position
                    FROM clients
                    WHERE position IS NOT NULL
                )
                ORDER BY RANDOM()
                LIMIT 1
            """
            row = await conn.fetchrow(query, learnable_disc_count, learn_level)

            if row:
                normalized = NormalizedPosition.from_bytes(row["position"])
                break

        if not normalized:
            # No jobs available for current disc count, tell client to wait
            return None

        job = Job(
            position=normalized.to_api(), level=get_learn_level(learnable_disc_count)
        )

        # Store the assigned job in client state
        await conn.execute(
            """
            UPDATE clients
            SET position = $1
            WHERE id = $2
            """,
            normalized.to_bytes(),
            client_id,
        )

        return job


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


@app.post("/api/positions/lookup")
async def get_positions(
    payload: LookupPositionsPayload,
    _: None = Depends(verify_auth),
    state: ServerState = Depends(get_server_state),
) -> list[SerializedEvaluation]:
    positions = [NormalizedPosition.from_api(pos) for pos in payload.positions]

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
        normalized = NormalizedPosition.from_bytes(row["position"])

        results.append(
            SerializedEvaluation(
                position=normalized.to_api(),
                level=row["level"],
                depth=row["depth"],
                confidence=row["confidence"],
                score=row["score"],
                best_moves=row["best_moves"],
            )
        )

    return results
