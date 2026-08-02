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

# The agent's extra system prompt lives in docs/sandbox.md (linked from
# CLAUDE.md), so it's one source of truth readable both here and in normal
# sessions. Read verbatim -- no shell expansion, same as the old quoted heredoc.
SYSTEM_PROMPT="$(cat docs/sandbox.md)"

SBX_BASE_NAME="${SBX_NAME:-flippy}"
SBX_MEMORY="${SBX_MEMORY:-4g}"
SBX_CPUS="${SBX_CPUS:-4}"

# Optional model override for the claude CLI (e.g. CLAUDE_MODEL=fable).
CLAUDE_MODEL_ARGS=()
if [ -n "${CLAUDE_MODEL:-}" ]; then
    CLAUDE_MODEL_ARGS=(--model "$CLAUDE_MODEL")
fi

# Multiple sandboxes can run concurrently, so pick the lowest free
# "$SBX_BASE_NAME-<n>" suffix instead of colliding on a shared name. Also
# skip any suffix with a leftover refs/sandboxes/<name>/* ref -- cleanup's
# `git fetch` leaves those behind even after `sbx rm`, since the ref isn't
# tied to the remote that fetched it, and reusing the name would step on it.
existing_names="$(sbx ls -q 2>/dev/null || true)"
existing_refs="$(git for-each-ref --format='%(refname)' refs/sandboxes | sed -E 's#^refs/sandboxes/([^/]+)/.*#\1#' | sort -u)"
n=1
while echo "$existing_names" | grep -qx "$SBX_BASE_NAME-$n" || echo "$existing_refs" | grep -qx "$SBX_BASE_NAME-$n"; do
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
    -- --append-system-prompt "$SYSTEM_PROMPT" "${CLAUDE_MODEL_ARGS[@]}"
