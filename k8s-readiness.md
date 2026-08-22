# Making flippy ready for the homelab k8s cluster

Self-contained list of changes to make in this repo *before* flippy is deployed on the
homelab k3s cluster. Each item states a **goal** that must be met and a **suggested
implementation**. The implementation is a starting point: deviate when project constraints
(style, existing abstractions, tests) make something else cleaner, as long as the goal holds.

## Target environment (for context)

- 27 × `rock-NN` agent nodes: **arm64** (RK3399: 2×A72 + 4×A53), 6 CPU, ~3.8 GiB RAM,
  ~28 GiB local disk each. One amd64 control-plane node (`potassium`) that flippy must not
  use.
- k3s + Argo CD, plain manifests, public registries only (no private registry, no pull
  secrets), hand-created Secrets, `local-path` storage (node-local, RWO).
- Planned layout: Postgres 17 StatefulSet (1 replica), Redis (1 replica, no persistence),
  **server Deployment with ≥2 replicas**, ~150 worker pods with `EDAX_TASKS=1` spread six
  per node, a migrations Job, and a daily `pg_dump` → S3 CronJob.
- One public hostname via ingress-nginx: `https://flippy.homelab.merpmerp.nl`. Workers talk
  to the server over an in-cluster ClusterIP service.

Items 1–3 are hard prerequisites; 4–9 are needed before the server runs with more than
one replica or the cluster is under real load.

---

## 1. Worker image contains edax, built for arm64

**Goal:** `docker run ghcr.io/lk16/flippy-worker:<tag>` on an arm64 host runs the worker
with a working edax and `eval.dat`, with no host bind mount. The same image (or a
multi-arch manifest) also works on amd64 for local use.

**Suggested implementation:**
- Extend `cmd/worker/Dockerfile` to a multi-stage build: a stage that clones a pinned edax
  commit (the okuhara fork currently used) and builds it with the NEON/ARM target, a Go
  build stage (`CGO_ENABLED=0 GOARCH` from buildx), and a minimal runtime stage that copies
  `bin/` and `data/eval.dat` to `/edax/...`.
- Set `EDAX_PATH` default to the baked-in path instead of `lEdax-x64`; drop the
  `EDAX_HOST_DIR` assumption from `docker-compose.yml` (or keep it as an optional override).
- Verify first, before anything else in this list, that edax's ARM build target actually
  compiles and produces correct results on A72/A53 — run the existing solver tests against
  it on a rock or under `qemu-user`. This is the biggest unknown.
- Build with `docker buildx --platform linux/arm64,linux/amd64` in CI (see item 2).

## 2. Server image, migrations image, published tags

**Goal:** Tagged, immutable images for the server and for running migrations exist in a
public registry, built automatically for arm64 (and amd64), so the cluster manifests can
pin a version.

**Suggested implementation:**
- `cmd/server/Dockerfile`: Go build with `CGO_ENABLED=0`; the `static/` and `wasm/` dirs
  are `go:embed`ed, so they only need to be in the build context. Runtime stage can be
  `gcr.io/distroless/static` or `alpine`. Expose 7777.
- Migrations: either a tiny image with `migrate` plus `migrations/` copied in, or add a
  `migrate` subcommand/flag to the server binary that runs golang-migrate programmatically
  and exits. The latter avoids a third image. The cluster will run this as an Argo
  `PreSync` hook Job.
- GitHub Actions workflow on push to `main` and on tags: `buildx` multi-arch, push to
  `ghcr.io/lk16/flippy-server` and `ghcr.io/lk16/flippy-worker`, tagged with the git sha
  and the tag name. No `latest` is required; the cluster pins shas.
- Set the commit via `-ldflags "-X main.commit=$SHA"` instead of shelling out to
  `git rev-parse HEAD` (`cmd/worker/main.go:27-33`), which always yields `unknown` in a
  container. Do the same for the server.

## 3. Authenticate the worker API

**Goal:** Only holders of a shared secret can fetch jobs, submit results, release claims,
or send heartbeats. Read-only browsing endpoints and the web UI remain open. Exposing the
server through an Ingress must not allow anyone on the LAN to poison the book.

**Suggested implementation:**
- Env var `FLIPPY_WORKER_TOKEN` on the server (required; refuse to start without it
  outside tests) and on the worker.
- Worker sends `Authorization: Bearer <token>` on every request. Server wraps
  `GET /api/jobs`, `POST /api/jobs/result`, `POST /api/jobs/release`,
  `POST /api/workers/heartbeat` in middleware that does a constant-time compare and returns
  401 otherwise. `POST /api/pgn` should probably be protected too, or at least
  rate-limited, since it creates DB rows.
- Keep `GET /api/boards`, `/api/stats`, `/api/workers`, `/api/level-config`, `/ws` and the
  HTML pages open.
- Deviation allowed: mTLS or a per-worker key are fine if preferred, but a single shared
  bearer token is sufficient for a home LAN and is trivial to inject from a k8s Secret.

## 4. Health and readiness endpoints

**Goal:** Kubernetes can tell (a) the process is alive, and (b) the server is able to serve
correct book data, i.e. the minimax cache has been built. A server whose cache is still
building must not receive traffic.

**Suggested implementation:**
- `GET /healthz` → 200 as soon as the HTTP listener is up. Cheap; no dependencies.
- `GET /readyz` → 200 only once the first `Cache.Rebuild` succeeded and Postgres + Redis
  pings succeed; 503 otherwise. Cache the dependency pings for a few seconds.
- Start the listener *before* the initial rebuild, so `/healthz` responds during startup
  (today `cache.Rebuild` runs before `ListenAndServe` in `cmd/server/main.go`). k8s will use
  a `startupProbe` on `/readyz` with a generous `failureThreshold`.
- Optionally `GET /version` returning the ldflags commit from item 2.

## 5. Cheapen the periodic book statistics

**Goal:** Keeping `book_stats` fresh must not cost a full scan of the `boards` table
(14 M rows and growing) every 60 s. The Postgres node is a 3.8 GiB SBC shared with workers;
the stats refresh should be a negligible fraction of one core on average.

**Suggested implementation:** any of
- Maintain per-`(disc_count, level)` counters incrementally: on every `SaveEvaluation`
  adjust the counters (Redis `HINCRBY` or a small `book_stats` table updated in the same
  transaction). Run the full `GROUP BY` only at startup / on demand to resync.
- Or: keep the query but run it every 10–15 min, and keep the `jobFloor` derivation
  (which depends on these stats) working on slightly stale data.
- Or: a Postgres materialized view refreshed `CONCURRENTLY` on the slower cadence.
The incremental counters are the best option if `jobFloor` needs near-real-time data.

## 6. Small container hygiene items

**Goal:** Nothing in the binaries assumes a developer checkout or the compose layout.

**Suggested implementation:**
- `.env` auto-loading (`internal/env/env.go`) is harmless when the file is absent; leave
  it, but make sure no code path *requires* a `.env`.
- Expose edax's `-hash-table-size` as a worker flag/env (`EDAX_HASH_TABLE_SIZE`), so
  per-pod memory can be tuned below the default ~113 MiB table when six workers share a
  3.8 GiB node.
- Ensure the worker exits non-zero on an unrecoverable edax failure rather than looping,
  so Kubernetes restarts the pod (backoff is free).
- Ensure `SIGTERM` handling in the worker kills the edax subprocess promptly and releases
  the claim within the 30 s grace period (currently `releaseTimeout = 5 s`, good).

---

## Multi-replica server

Items 7–9 remove the assumptions that make the server a singleton. After these, the server
can run as a Deployment with `replicas ≥ 2` and rolling updates; Redis remains the single
shared state store (it is already used for claims, heartbeats, priority jobs and analysis
results).

## 7. Websocket liveness shared across replicas

**Goal:** Whether a browser's websocket connection is still open must be answerable by any
server replica, not only the one holding the socket, so interactive/priority analysis jobs
are dequeued (or dropped) correctly regardless of which replica a worker talks to. A
crashed replica must not leave "live" connections behind forever.

**Suggested implementation:**
- Replace the in-process counter in `internal/api/server.go` (`liveConns`, `lastConnID`,
  `connsMu`) and `websocket.go` (`registerConn`/`unregisterConn`/`connLive`) with Redis:
  - connection ID: random 16-byte hex (same helper as worker IDs), so two replicas can't
    collide;
  - `registerConn`: `SET conn:<id> 1 EX 90`;
  - the websocket read loop refreshes the TTL (`EXPIRE`) every ~30 s, or on each incoming
    client message if those are frequent enough;
  - `unregisterConn`: `DEL conn:<id>`;
  - `connLive`: `EXISTS conn:<id>`.
- Check how analysis results reach the websocket. If the handler waits on an in-process
  channel that the result-submission handler writes to, that breaks across replicas:
  switch to polling `analysis:<position>` in Redis (already written there with a 30-min
  TTL) or to Redis pub/sub on a per-position channel.
- The `priority_jobs` list and `priority_pending` set already live in Redis; no change.

## 8. Book cache invalidation across replicas

**Goal:** When a leaf-disc-count (`book.LeafDiscs`) evaluation is saved, every server
replica rebuilds its minimax cache within a few seconds, without the rebuild blocking the
request that triggered it. Readiness reflects whether a replica's cache has been built at
least once.

**Suggested implementation:**
- Add a Redis key `book:version` (integer). In `handleSubmitJobResult`
  (`internal/api/handlers.go`, the `discCount == book.LeafDiscs` branch) replace the inline
  `s.cache.Rebuild(r.Context())` with `INCR book:version`.
- Each server runs a goroutine that polls `GET book:version` every 2–5 s (pub/sub is also
  fine, but polling is simpler and also covers a replica that started while Redis was
  briefly unavailable). On change, call `Rebuild` with `context.Background()` plus a
  timeout, then record the version it built. Coalesce: if the version changed again during
  a rebuild, rebuild once more, not N times.
- `Cache` must stay safe for concurrent `Get` during `Rebuild` (swap the map under the
  existing lock after building the new one, which is how it appears to work already).
- `/readyz` (item 4) returns 200 only after the first successful rebuild.
- Bonus: the result-submission request no longer stalls all workers for the duration of a
  full 12-disc scan.

## 9. Single runner for the periodic stats refresh

**Goal:** The heavy stats query from item 5 (or its replacement) runs once per interval
cluster-wide, not once per replica, while still running if the replica that usually does it
dies.

**Suggested implementation:**
- In `RunBookStatsRefresh`, before each run: `SET book_stats:lock <replica-id> NX EX <interval-5s>`.
  Only the replica that wins the `SET` executes the refresh and writes the `book_stats`
  hash; the others skip this tick. No leader election library needed; if the holder dies
  the lock expires and another replica picks it up on the next tick.
- `replica-id` can be the pod hostname or a random ID at startup; it is only for logging.
- All replicas continue to *read* `book_stats` from Redis as today.

---

## What the cluster side will do once these land

(For reference only; implemented in the homelab repo, not here.)

- Server: Deployment, `replicas: 2`, `RollingUpdate`, spread across nodes, probes on
  `/healthz` and `/readyz`, env from a Secret (`FLIPPY_POSTGRES_URL`, `FLIPPY_REDIS_URL`,
  `FLIPPY_WORKER_TOKEN`).
- Workers: Deployment, ~150 replicas, `EDAX_TASKS=1`, `requests: cpu 900m / memory 200Mi`,
  `limits: memory 300Mi`, `topologySpreadConstraints` on hostname → six per node.
- Migrations: Argo `PreSync` hook Job using the migrations image/subcommand.
- Postgres: StatefulSet on one pinned rock, 20 GiB `local-path` PVC.
- Backup: daily CronJob, `pg_dump -Fc` → `aws s3 cp`, own IAM user with `PutObject` only,
  retention via S3 lifecycle rule.
