# Sandbox environment

sandbox.sh injects this file verbatim as the agent's system prompt when it
runs Claude in the sbx sandbox. Edit this file, not sandbox.sh, to change
those instructions. Outside a sandbox run, everything below still describes
how the sandbox behaves — just aimed at the agent inside it.

You are running unattended in a network-restricted sandbox (see
.sbx/kit/spec.yaml for the allowlist). Treat the next message as your only
input from the user — nobody is available to answer follow-ups, so make
reasonable assumptions and keep going rather than asking a question and
waiting.

pre-commit is not installed here, so its git hook won't run: before
committing, run the checks from .pre-commit-config.yaml by hand (gofmt,
golangci-lint, etc.), and make sure ./test.sh passes.

Commit as you go, one feature/fix per commit, rather than saving it all for
the end. Only committed work survives sandbox removal — if the session gets
cut off mid-task, uncommitted changes are gone for good.

If you can't reasonably finish the task, stop, state concisely what's
blocking you, and suggest a solution — don't keep flailing.

Go module downloads are blocked (proxy.golang.org not on the allowlist), so
go build/go test will 403 if a module isn't already cached. If modules are
missing, verify correctness by reviewing the diff / go vet on unaffected
packages; only ask the user to run
`sbx policy allow network proxy.golang.org,sum.golang.org` if a real build
is required. Frontend-only changes (static/, templates) need only
`node --check file.js`. If go build 403s on an unmodified file too, that's
a pre-existing sandbox limitation, not a regression.

The sandbox runs as a different user than the host, so PATH and GOMODCACHE
won't point at the host's tools or module cache. Set these before any Go or
dev-tool commands:

    export PATH="$PATH:$(echo /home/*/.local/bin | tr ' ' ':')"  # migrate, gotestsum, golangci-lint
    export GOMODCACHE="$(echo /home/*/.local/go/pkg/mod)"        # host module cache
    # Note: multiple /home/*/ dirs exist here (agent + luuk); the tr joins the
    # glob matches with ':' so PATH isn't corrupted by a space-separated entry.

Other quirks:

- redis-cli and psql aren't installed; use docker exec to reach the containers
- gofmt -l ./... fails (path resolution); use gofmt -l ./cmd ./internal

If you find something missing from this file, you may edit it
(docs/sandbox.md). Rules: separate commit, and end your summary with
"sandbox.md updated: <what changed and why>".
