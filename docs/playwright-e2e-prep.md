# Playwright e2e tests — host preparation

What a *future* sandbox session needs prepared on the host before it can write
and run Playwright regression tests against the frontend. This file is only
about that prep; it deliberately says nothing about which flows to test.

Hard constraint: **no new allowlisted domains.** `registry.npmjs.org` and
`cdn.playwright.dev` stay blocked, so everything Playwright would normally
download must be fetched on the host and mounted read-only into the sandbox.

Mount mechanics, for reference: `sandbox.sh` passes mounts positionally to
`sbx run claude . <path>:ro …`, and a mounted host directory appears inside the
sandbox at **the same absolute path** (`/home/luuk/.cargo`, `/home/luuk/.local/bin`,
… — verified in a session). Each new item below therefore means one extra
`:ro` entry in `sandbox.sh`'s mount list. Inside the sandbox, resolve them by
glob (`/home/*/…`), the way `docs/sandbox.md` already does for cargo and Go —
the sandbox user is not the host user.

## Already available — no prep needed

- **Node 22** (`v22.22.1`) and `npm` (9.2.0), matching CI's `node-version: "22"`.
  Pin to 22; the Node-26 question in the old scratch notes is moot, since
  bringing 26 in would need a blocked domain.
- **Postgres 17 / Redis 8** images — Docker Hub is already allowlisted
  (`.sbx/kit/spec.yaml`), and `docker compose` works in-sandbox.
- **`migrate`, Go module cache, cargo/rustup, the Edax checkout** — already
  mounted by `sandbox.sh`.
- **`apt` from `archive.ubuntu.com` / `security.ubuntu.com`** — allowlisted on
  :443 (see the sources-list https caveat in `docs/sandbox.md`).
- **The wasm evaluator** — `wasm/edax-eval/dist/` is committed, so the
  client-side evaluation path needs no build and no network.

## 1. The `@playwright/test` package

The next session will add a net-new `package.json` + `package-lock.json`
(there is none today) pinning an exact Playwright version — call it `X` below.
`X` must be identical everywhere: package, browser download, and any CI job.

Pick one of these; **option A** is preferred because the sandbox ends up with a
normal `node_modules` inside the repo clone.

**A. Mount the host npm cache, install offline in-sandbox.**

- Host, once: run `npm install -D @playwright/test@X` (or `npm ci` after the
  lockfile exists) in a checkout of this repo, so `~/.npm/_cacache` holds every
  tarball the lockfile references.
- Host: add `"$HOME/.npm":ro` to `sandbox.sh`'s mount list.
- Sandbox, per session: the cache must be writable, so copy it first, then
  install strictly offline:
  ```
  cp -a /home/*/.npm ~/.npm-offline
  npm ci --offline --cache ~/.npm-offline
  ```
  with `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1` set, so the package's postinstall
  does not try to fetch browsers (those come from §2).
- Caveat to check on the host: the sandbox's npm is **9.2.0**. If the host's
  npm writes a newer `lockfileVersion` than that can consume, either generate
  the lockfile with a matching npm on the host, or use option B.

**B. Mount a prebuilt `node_modules`.**

- Host, once: `npm install -D @playwright/test@X` into a dedicated directory,
  e.g. `~/.cache/flippy-e2e/`, and mount `~/.cache/flippy-e2e:ro`.
- Sandbox: `export NODE_PATH=/home/*/.cache/flippy-e2e/node_modules` and put
  `…/node_modules/.bin` on `PATH`.
- Simpler, but the tree is read-only and lives outside the repo, so anything
  that wants to write into `node_modules` (or resolve it the normal way, from
  the config file's directory) has to be worked around.

## 2. Chromium binaries

- Host, once, with the **same version `X`**, because the browser revision is
  pinned per Playwright release:
  ```
  PLAYWRIGHT_BROWSERS_PATH="$HOME/.cache/ms-playwright" npx playwright@X install chromium
  ```
  (Add `firefox`/`webkit` only if cross-browser coverage is actually wanted —
  each is a few hundred MB of mount.)
- Host: mount `"$HOME/.cache/ms-playwright":ro`.
- Sandbox: `export PLAYWRIGHT_BROWSERS_PATH=/home/*/.cache/ms-playwright` and
  `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1`.
- Two things to verify in the first session, both cheap to work around:
  - **Read-only mount**: running a browser from a ro directory should be fine;
    if Playwright insists on writing there (marker/`.links` files), copy it to
    a writable path in-session (~400 MB, well inside the 10 GiB root).
  - **Distro match**: the sandbox is **Ubuntu 26.04**. Playwright picks its
    build from the *host's* distro. If the host is on something older, the
    downloaded Chromium may still be fine, but confirm it launches before
    building tests on top of it (`npx playwright cr --version`-style smoke).

## 3. Chromium's system libraries

Chromium needs `libnss3`, `libatk`, fonts, etc. This is `apt`, which is
allowlisted, so it can happen in-sandbox — but it should be *declared*, not
improvised per session:

- Host, once: `npx playwright install-deps --dry-run chromium` prints the exact
  apt package list for the target Ubuntu release.
- Host: paste that list into `.sbx/kit/spec.yaml`'s startup commands, next to
  the existing `gcc libc6-dev` install, so every sandbox boots with them.
- If the apt route is ever refused, the offline fallback is `apt-get download`
  of those packages on the host into a mounted directory plus `dpkg -i`
  in-session — more fragile, only if needed.

## 4. Sandbox resources

- **`/dev/shm` is 64 MB** (measured). Chromium crashes or flakes badly at that
  size. Either give the sandbox a bigger shm (check whether `sbx run` exposes a
  tmpfs/shm-size flag on the host; if so, add it to `sandbox.sh`), or launch
  Chromium with `--disable-dev-shm-usage` in `playwright.config.js`. Decide on
  the host side, since only the first option needs prep.
- **Memory**: `sandbox.sh` defaults to `SBX_MEMORY=4g`, and the session already
  showed ~3 GiB usable. Postgres + Redis + the Go server + Chromium wants more
  headroom: run `SBX_MEMORY=8g ./sandbox.sh` for e2e sessions.
- **Disk**: browsers (~400 MB) + `node_modules` (~100 MB) fit comfortably in
  the 10 GiB root default.

## 5. Data the app serves

- **Book positions** need no prep: `go run ./cmd/loader seed` computes them in
  code (`othello.PrecomputedBoards12`), no fixture file involved.
- **Evaluations are a decision the host has to make**, because a freshly seeded
  database has boards but no scores:
  - If the tests should exercise the *server-answered* rendering path, the
    sandbox needs either (a) a worker with a **built** Edax binary — the
    `EDAX_HOST_DIR` checkout is already mounted, but it must be compiled on the
    host and `EDAX_PATH` set in `.env` — or (b) a SQL fixture of an
    already-evaluated book (`archive_book.sh` produces exactly such a dump)
    mounted `:ro` and restored after `migrate`.
  - If the tests only exercise the *local wasm* path (scores appearing under
    every move without any server result), neither is needed — that path is
    fully client-side.
- Fixture size matters more than realism here: a dump small enough to restore
  in a few seconds per run beats the full book.

## 6. Optional: pin the Docker images offline

Docker Hub is allowlisted, so this is only a speed/insurance measure:
`docker save postgres:17 redis:8 | gzip > ~/.cache/flippy-e2e/images.tar.gz` on
the host, mount it, `docker load` in-session.

## 7. Getting results back out

`sandbox.sh` runs with `--clone`: only **committed** work survives. Traces,
screenshots and HTML reports are binary and do not belong in git, so plan to
pull them out with `sbx cp <sandbox>:<path> .` from the host instead — and
configure `trace`/`screenshot` as retain-on-failure so there is little to pull
in the common case.

## 8. First-session verification checklist

Run these before writing any test; each maps to one prep item above:

```
node --version                                   # v22.x
ls /home/*/.npm/_cacache >/dev/null              # §1 option A
ls /home/*/.cache/ms-playwright                  # §2, chromium-<rev> present
dpkg -l | grep -c libnss3                        # §3
df -h /dev/shm                                   # §4, >64M or plan the flag
free -g                                          # §4
echo "$EDAX_PATH" && ls "$EDAX_PATH"             # §5, only if server evals are in scope
docker compose -f docker-compose.yml up -d --wait postgres redis   # app stack
```

## 9. What the sandbox session will add to the repo

So the host knows what to expect back: `package.json`, `package-lock.json`,
`playwright.config.js`, a test directory, `.gitignore` entries for
`node_modules`/`test-results`, and wiring into `test.sh`. CI is *not* subject
to the offline constraint — a GitHub runner can install Playwright and its
browsers normally — so the CI job, if we add one, looks nothing like the
sandbox setup above.
