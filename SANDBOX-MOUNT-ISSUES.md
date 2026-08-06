# Sandbox read-only mount audit

Audit run 2026-08-06 inside an `sbx` sandbox at commit `1037f93` (clone mode).

## Result

All 8 read-only mounts are present, populated and functional:

| Mount | Purpose |
| --- | --- |
| `/home/luuk/.local/go/pkg/mod` | host Go module cache |
| `/home/luuk/.local/bin` | migrate, gotestsum, golangci-lint |
| `/usr/local/go` | Go toolchain (go1.26.0) |
| `/home/luuk/.cargo` | host crate cache + cargo/rustc binaries |
| `/home/luuk/.rustup` | toolchains, incl. wasm32-unknown-unknown target |
| `/home/luuk/projects/INACTIEF/edax-reversi` | edax binaries for Process tests |
| `/run/sandbox/source` | host repo (clone mode) |
| `/etc/resolv.conf`, `/etc/hosts` | DNS/network config |

Verified working: `go build ./...` and `go vet ./internal/...` exit 0,
`golangci-lint run ./...` reports 0 issues, `gofmt -l ./cmd ./internal` is
clean, and `cargo check` succeeds for both the native and
`wasm32-unknown-unknown` targets against the read-only `CARGO_HOME` /
`RUSTUP_HOME` (crc32fast's build script links, so gcc is in place).

The problems below are gaps around the mounts, not broken mounts.

## Problems

### 1. `wasm/` is missing from the sandbox working tree

`/home/luuk/projects/flippy/wasm` does not exist. On the host the directory
is git-*staged but never committed* — `git status` in `/run/sandbox/source`
shows 20+ `A ` entries under `wasm/edax-eval/` — and clone mode only copies
committed state.

So `docs/sandbox.md`'s whole Rust/cargo section, which calls
`wasm/edax-eval` "the only Rust crate in this repo", points at a directory
no sandbox session can reach, and the `.cargo` / `.rustup` mounts have
nothing to build. The crate itself is fine: copying it out of
`/run/sandbox/source` into a scratch dir and running `cargo check` works.

Fix: commit `wasm/` on the host.

### 2. Read-only `GOMODCACHE` makes every Go command print a warning

Every `go build` / `go vet` emits on stderr:

```
go: writing stat cache: open /home/luuk/.local/go/pkg/mod/cache/download/github.com/lk16/flippy/@v/v0.0.0-...tmp: read-only file system
```

The command still exits 0, but it reads like a failure. Not mentioned in
`docs/sandbox.md`.

### 3. `.env` is missing, so `EDAX_PATH` is unset

The edax mount is correct — `/home/luuk/projects/INACTIEF/edax-reversi/bin/lEdax-x64`
exists, is executable, and matches the host `.env` value exactly. But `.env`
is untracked on the host, so it is not in the clone, and nothing injects
`EDAX_PATH` into the sandbox environment. The `internal/edax` subprocess
tests therefore skip instead of running.

### 4. Project-level `.claude/` is missing

`/run/sandbox/source/.claude` exists on the host but is untracked and not
gitignored, so it is absent from the clone. Any project-scoped Claude
settings, hooks or permissions do not apply inside the sandbox.
