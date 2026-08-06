#!/usr/bin/env bash
# Runs Claude Code inside a docker sandbox (sbx)
#
# Features:
# - enforce limit on mem/cpu/disk usage
# - clones git repo to sandbox
# - supports multiple claude setups on same machine
# - supports running multiple sandboxes for same project - no name collision
# - removes sbx on claude exit
# - pulls but prevents losing work
# - mounts edax-reversi checkout read-only for Process tests


set -euo pipefail

# Run from repo root.
cd "$(dirname "${BASH_SOURCE[0]}")"

# Load local dev config
if [ -f .env ]; then
    set -a
    # shellcheck disable=SC1091
    source .env
    set +a
fi

: "${EDAX_HOST_DIR:?EDAX_HOST_DIR must be set in .env (path to the edax-reversi checkout)}"

if [ -z "${CLAUDE_OAUTH_TOKEN_FILE:-}" ]; then
    echo "CLAUDE_OAUTH_TOKEN_FILE is not set." >&2
    echo "Set it up once:" >&2
    echo "  1. Run: claude setup-token" >&2
    echo "  2. Save the printed token to a file, e.g. ~/.secrets/claude-oauth-work.token" >&2
    echo "  3. Export CLAUDE_OAUTH_TOKEN_FILE to point at that file (e.g. via direnv)." >&2
    exit 1
fi

if [ -n "${CLAUDE_OAUTH_TOKEN_FILE:-}" ] && [ ! -s "$CLAUDE_OAUTH_TOKEN_FILE" ]; then
    echo "CLAUDE_OAUTH_TOKEN_FILE ($CLAUDE_OAUTH_TOKEN_FILE) does not exist or is empty." >&2
    exit 1
fi

# Go module cache
GOMODCACHE_HOST="$(go env GOMODCACHE)"

# Mount host go toolchain folder to ensure correct golang version.
GOROOT_HOST="$(go env GOROOT)"

# Go installed binaries (used for: migrate, gotestsum, golangci-lint)
GOBIN_HOST="$(go env GOBIN)"

CARGO_HOME_HOST="$HOME/.cargo"
RUSTUP_HOME_HOST="$HOME/.rustup"


# Load system prompt, linked from CLAUDE.md, one source of truth.
SYSTEM_PROMPT="$(cat docs/sandbox.md)"

# Limit CPU and memory usage.
SBX_MEMORY="${SBX_MEMORY:-4g}"
SBX_CPUS="${SBX_CPUS:-4}"

# Limit disk usage.
export DOCKER_SANDBOXES_ROOT_SIZE="${SBX_ROOT_SIZE:-10g}"
export DOCKER_SANDBOXES_DOCKER_SIZE="${SBX_DOCKER_SIZE:-10g}"

# Override selected model for claude CLI.
CLAUDE_MODEL_ARGS=()
if [ -n "${CLAUDE_MODEL:-}" ]; then
    CLAUDE_MODEL_ARGS=(--model "$CLAUDE_MODEL")
fi

# Sandbox base name - use project name in kebab-case
SBX_BASE_NAME='flippy'

# Find first <sandbox name>-<n> that is not running and has no git refs.
existing_names="$(sbx ls -q 2>/dev/null || true)"
existing_refs="$(git for-each-ref --format='%(refname)' refs/sandboxes | sed -E 's#^refs/sandboxes/([^/]+)/.*#\1#' | sort -u)"
n=1
while echo "$existing_names" | grep -qx "$SBX_BASE_NAME-$n" || echo "$existing_refs" | grep -qx "$SBX_BASE_NAME-$n"; do
    n=$((n + 1))
done
SBX_NAME="$SBX_BASE_NAME-$n"

cleanup() {
    # Pull all committed work from sandbox
    git fetch "sandbox-$SBX_NAME" || true

    # Prevent losing work if any change is not committed.
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

    # Remove secret for this sandbox name
    sbx secret rm "$SBX_NAME" --host api.anthropic.com -f 2>/dev/null || true

    # Remove sandbox.
    sbx rm --force "$SBX_NAME"
}

# Cleanup at script exit, when user closes claude prompt inside sandbox.
trap cleanup EXIT

sbx create claude \
    . \
    "$GOMODCACHE_HOST":ro \
    "$GOBIN_HOST":ro \
    "$GOROOT_HOST":ro \
    "$CARGO_HOME_HOST":ro \
    "$RUSTUP_HOME_HOST":ro \
    "$EDAX_HOST_DIR":ro \
    --clone \
    --name "$SBX_NAME" \
    --kit .sbx/kit \
    --memory "$SBX_MEMORY" \
    --cpus "$SBX_CPUS"

# Clear stale secret from a previous run under this name
sbx secret rm "$SBX_NAME" --host api.anthropic.com -f 2>/dev/null || true

# Set claude token for this sandbox.
tr -d '\n' <"$CLAUDE_OAUTH_TOKEN_FILE" | sbx secret set-custom "$SBX_NAME" --host api.anthropic.com --env CLAUDE_CODE_OAUTH_TOKEN

# Run claude
sbx run claude --name "$SBX_NAME" \
    -- --append-system-prompt "$SYSTEM_PROMPT" "${CLAUDE_MODEL_ARGS[@]}"
