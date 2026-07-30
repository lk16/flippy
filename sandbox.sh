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
    git fetch "sandbox-$SBX_NAME" || true
    sbx rm --force "$SBX_NAME"

    # sbx removes the "sandbox-$SBX_NAME" remote and its refs/remotes/*
    # tracking refs on teardown, but not refs/sandboxes/<name>/* (the
    # --clone refspec's other target) -- delete those ourselves so a reused
    # numeric name doesn't accumulate orphaned refs from past runs.
    git for-each-ref --format='%(refname)' "refs/sandboxes/$SBX_NAME/" |
        xargs -r -n1 git update-ref -d
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
    --cpus "$SBX_CPUS"
