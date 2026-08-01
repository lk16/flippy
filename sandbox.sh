#!/usr/bin/env bash
# Runs Claude Code inside an sbx sandbox for this project: real Docker access
# (so test.sh's docker-compose.test.yml works), network locked down to
# .sbx/kit's allowlist, and CPU/memory/disk capped explicitly. Torn down via
# `sbx rm` on exit (see cleanup trap below), so each run starts fresh.
#
# --clone: the agent works on a private in-container clone of this repo, not
# the real working tree (which is mounted read-only instead). Commits land on
# a `sandbox-<name>` git remote, fetched back out explicitly once reviewed.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

# Load local dev config (e.g. EDAX_HOST_DIR), same pattern as local.sh/test.sh.
if [ -f .env ]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
fi

: "${EDAX_HOST_DIR:?EDAX_HOST_DIR must be set in .env (path to the edax-reversi checkout)}"

# Resolved dynamically rather than hardcoded, so this script stays portable
# across machines with different usernames/homedirs.
GOMODCACHE_HOST="$(go env GOMODCACHE)"
LOCAL_BIN_HOST="$HOME/.local/bin"

# Sets expectations the agent can't otherwise infer: nobody's watching this
# session, so it should act on whatever the user's first message says instead
# of pausing to ask; the git hook won't fire since pre-commit itself isn't
# installed in the sandbox, so hooks from .pre-commit-config.yaml need running
# by hand; and ./test.sh must pass before anything gets committed.
SYSTEM_PROMPT="$(cat <<'EOF'
You are running unattended in a network-restricted sandbox (see .sbx/kit/spec.yaml
for the allowlist). Treat the next message as your only input from the user --
nobody is available to answer follow-ups, so make reasonable assumptions and
keep going rather than asking a question and waiting.

pre-commit is not installed here, so its git hook won't run: before committing,
run the checks from .pre-commit-config.yaml by hand (gofmt, golangci-lint, etc.),
and make sure ./test.sh passes.

Commit as you go, one feature/fix per commit, rather than saving it all for
the end. Only committed work survives sandbox removal -- if the session gets
cut off mid-task, uncommitted changes are gone for good.

If you can't reasonably finish the task, stop, state concisely what's blocking
you, and suggest a solution -- don't keep flailing.

Go module downloads are blocked (proxy.golang.org not on the allowlist), so go
build/go test will 403 if a module isn't already cached. If modules are missing,
verify correctness by reviewing the diff / go vet on unaffected packages; only ask
the user to run `sbx policy allow network proxy.golang.org,sum.golang.org` if a
real build is required. Frontend-only changes (static/, templates) need only
`node --check file.js`. If go build 403s on an unmodified file too, that's a
pre-existing sandbox limitation, not a regression.

The sandbox runs as a different user than the host, so PATH and GOMODCACHE won't
point at the host's tools or module cache. Set these before any Go or dev-tool
commands:
  export PATH="$PATH:$(echo /home/*/.local/bin)"         # migrate, gotestsum, golangci-lint
  export GOMODCACHE="$(echo /home/*/.local/go/pkg/mod)"  # host module cache

Other quirks:
- redis-cli and psql aren't installed; use docker exec to reach the containers
- gofmt -l ./... fails (path resolution); use gofmt -l ./cmd ./internal

If you find something missing from this prompt, you may edit sandbox.sh to add it.
Rules: separate commit, and end your summary with "sandbox.sh updated: <what changed and why>".
EOF
)"

SBX_BASE_NAME="${SBX_NAME:-flippy}"
SBX_MEMORY="${SBX_MEMORY:-4g}"
SBX_CPUS="${SBX_CPUS:-4}"

# Multiple sandboxes can run concurrently, so pick the lowest free
# "$SBX_BASE_NAME-<n>" suffix instead of colliding on a shared name.
existing_names="$(sbx ls -q 2>/dev/null || true)"
n=1
while echo "$existing_names" | grep -qx "$SBX_BASE_NAME-$n"; do
    n=$((n + 1))
done
SBX_NAME="$SBX_BASE_NAME-$n"

# Disk defaults to 20 GiB/sandbox and is uncapped across concurrently running
# sandboxes otherwise -- tighten explicitly. See
# work/notes/2026-07-29_sbx_isolation_findings.md for how this was verified.
export DOCKER_SANDBOXES_ROOT_SIZE="${SBX_ROOT_SIZE:-10g}"
export DOCKER_SANDBOXES_DOCKER_SIZE="${SBX_DOCKER_SIZE:-10g}"

cleanup() {
    # Preserves committed work, but git fetch can't do anything about
    # uncommitted changes in the agent's clone -- e.g. if the session was
    # interrupted before it got to commit. Check for that separately and
    # refuse to destroy the sandbox in that case, rather than silently
    # losing work.
    git fetch "sandbox-$SBX_NAME" || true

    local dirty
    dirty="$(sbx exec "$SBX_NAME" git -C "$PWD" status --porcelain 2>/dev/null || true)"
    if [ -n "$dirty" ]; then
        echo "WARNING: sandbox $SBX_NAME has uncommitted changes -- not removing it." >&2
        echo "$dirty" >&2
        echo "Inspect:  sbx exec $SBX_NAME git -C $PWD diff" >&2
        echo "Recover:  sbx cp $SBX_NAME:$PWD/<file> ." >&2
        echo "Then remove manually once safe: sbx rm --force $SBX_NAME" >&2
        return
    fi

    sbx rm --force "$SBX_NAME"
}
trap cleanup EXIT

sbx run claude \
    . \
    "$GOMODCACHE_HOST":ro \
    "$LOCAL_BIN_HOST":ro \
    "$EDAX_HOST_DIR":ro \
    --clone \
    --name "$SBX_NAME" \
    --kit .sbx/kit \
    --memory "$SBX_MEMORY" \
    --cpus "$SBX_CPUS" \
    -- --append-system-prompt "$SYSTEM_PROMPT"
