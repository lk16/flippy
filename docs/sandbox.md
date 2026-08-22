# Sandbox environment

pre-commit is not installed here, so its git hook won't run: before
committing, run the checks from .pre-commit-config.yaml by hand (gofmt,
golangci-lint, etc.), and make sure ./test.sh passes.

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
    # The host module cache is mounted read-only, and go needs to write into
    # GOMODCACHE (stat cache, lock files) — pointing GOMODCACHE at it fails with
    # "read-only file system". Instead use a writable cache and serve the host's
    # downloaded modules through GOPROXY's file:// form:
    export GOMODCACHE="$HOME/go/pkg/mod"
    export GOPROXY="file://$(echo /home/*/.local/go/pkg/mod/cache/download)"
    export GOSUMDB=off   # sum.golang.org is blocked; hashes still checked against go.sum
    # Note: multiple /home/*/ dirs exist here (agent + luuk); the tr joins the
    # glob matches with ':' so PATH isn't corrupted by a space-separated entry.
    # Only module versions the host has actually downloaded are servable: adding a
    # NEW dependency usually dies on a missing .mod/.zip somewhere in the graph
    # (fetch it on the host first, or pick an approach without new deps).

Same deal for Rust/cargo (used by wasm/edax-eval, the only Rust crate in this
repo): set these before any cargo/rustup commands:

    export PATH="$PATH:$(echo /home/*/.cargo/bin | tr ' ' ':')"
    export CARGO_HOME="$(echo /home/*/.cargo)"    # host crate cache
    export RUSTUP_HOME="$(echo /home/*/.rustup)"  # host toolchains + targets

crates.io and static.rust-lang.org are not on the network allowlist, so
`cargo build` here only works against crates already cached in
CARGO_HOME/registry, and `rustup target add` can't fetch a new target
(wasm32-unknown-unknown is already installed on the host). If a crate or
target is missing, run `cargo fetch` / `rustup target add <target>` on the
host first, then retry in the sandbox.

gcc and libc6-dev are installed via a kit startup command
(.sbx/kit/spec.yaml), which also allowlists archive.ubuntu.com and
security.ubuntu.com for the apt install. Don't assume it succeeded — check
`which gcc` before blaming a build error on something else. In a sandbox
created before 2026-08-05 the startup install silently failed (its apt-get
hit the http:// sources described below and 403'd on every retry), leaving
no compiler behind; the fix is the same sed, run by hand. This gives `cargo test`/`cargo
build` for the native target (x86_64-unknown-linux-gnu) a working linker,
and also fixes `cargo check`/`clippy` for crates with a `build.rs` anywhere
in their dependency graph — a build script is always compiled and linked as
a native host binary regardless of the crate's own target (so it used to
break even `--target wasm32-unknown-unknown` runs). Confirmed hands-on with
`flate2`: `flate2` and `miniz_oxide` themselves have no `build.rs`, but the
transitive dep `crc32fast` does (just to emit a rustc-version cfg flag, no
actual C code) — that alone used to fail `linker `cc` not found` on plain
`cargo check` before gcc/libc6-dev were installed. `cargo build --target
wasm32-unknown-unknown` also works for crates without build scripts, since
wasm doesn't need a system linker there.

That apt install can itself 403 even though archive.ubuntu.com/
security.ubuntu.com are allowlisted: the allowlist only covers port 443, but
`/etc/apt/sources.list.d/ubuntu.sources` defaults to `http://` (port 80) URIs
on this base image. If `apt-get update`/`install` fails with "403 Forbidden"
pointing at port 80 (check with `curl -i http://archive.ubuntu.com/...`,
which returns the sandbox's own policy-block body, not a real Ubuntu mirror
response), switch those URIs to `https://` and retry:

    sudo sed -i 's|http://archive.ubuntu.com|https://archive.ubuntu.com|; s|http://security.ubuntu.com|https://security.ubuntu.com|' /etc/apt/sources.list.d/ubuntu.sources

Other quirks:

- redis-cli and psql aren't installed; use docker exec to reach the containers
- gofmt -l ./... fails (path resolution); use gofmt -l ./cmd ./internal
- docker can pull Docker Hub images (docker.io is allowed), but network
  *inside* `docker build` containers hits the default-deny policy: apt against
  deb.debian.org, go mod download, and curl to github.com all 403. Full image
  builds therefore fail here even when the Dockerfile is correct — verify the
  pieces outside docker (gcc/qemu-user for C, GOOS/GOARCH cross-builds for Go)
  and let CI do the real build.
- apt over https://archive.ubuntu.com works (see above), so host-side
  cross-verification tooling is installable: gcc-aarch64-linux-gnu +
  libc6-dev-arm64-cross + qemu-user compile and run arm64 binaries
  (`qemu-aarch64 -L /usr/aarch64-linux-gnu <binary>`).

If you find something missing from this file, you may edit it
(docs/sandbox.md). Rules: separate commit, and end your summary with
"sandbox.md updated: <what changed and why>".
