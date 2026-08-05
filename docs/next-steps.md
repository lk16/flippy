# Next steps

Features and fixes we don't have yet. Mostly behavior from the pre-rewrite
implementation (`origin/main`) that was deliberately left out; none of it
is required — pick items up when a real need appears.

## Verification still owed

- Manual smoke test: worker + server + edax end-to-end on a few jobs.
- Manual smoke test: frontend pages (game, stats, clients) in a browser.

## Features

- **Auth**: no request auth at all. Old had X-Token/Basic Auth middleware
  on the API and separate Basic Auth on the HTML admin pages.
- **`GET /version`**: no version/build-info endpoint.
- **Stricter evaluation validation**: submitted results check
  level/depth/score bounds, but not the confidence enum
  ({73,87,95,98,99,100}) or an explicit level floor (`TargetLevel` never
  assigns below 28, so the floor holds by construction, not by input
  validation).
- **PGN illegal-move tolerance**: old auto-inserted a pass when a recorded
  move was illegal, recovering from bad data; our parser errors instead.
  Only matters if a real-world PGN turns up that needs it.
- **Desktop GUI**: no `cmd/gui` (Go raylib) or pygame equivalent —
  free-play board, PGN stepping with alt-move exploration, move-frequency
  stats, live-game screen-scraping.
- **CLI tools**: incremental PGN folder-watch importer (last-processed
  tracking), `book validate` (recompute disc counts, re-check evaluation
  invariants), `pgn_organizer` (sort PGNs by date/variant, download from
  playok.com/Othello Quest), `recent_games`, `show_board`.
  (`pgn_analyzer`'s job is covered by the web PGN-analysis page.)
- **`FLIPPY_EDAX_VERBOSE`**: no debug flag to dump edax
  command/cwd/stdin/stdout.
- **Config niceties**: no `PROJECT_ROOT` placeholder substitution in env
  vars, no `LOG_LEVEL` verbosity control.

## Testing gaps

- The websocket client's reconnect/queueing logic has no JS unit tests;
  `static/test/` covers board logic only.
- No browser-level regression tests at all: nothing catches a frontend
  wiring bug (e.g. evaluations not appearing under the legal moves) short
  of opening the page by hand. Playwright is the intended answer;
  [playwright-e2e-prep.md](playwright-e2e-prep.md) lists what the host has
  to prepare first, since the sandbox can't reach npm or Playwright's CDN.

## Build artifacts

- **`wasm/edax-eval/dist/`** (`edax_eval.wasm`, `weights.bin.gz`,
  `weights_manifest.json`) is committed to git — unlike `generated/` and
  `target/` (both gitignored scratch output), these are the actual files
  `internal/web` embeds and serves at `/static/wasm/`, so the running
  server has no build step of its own to reproduce them. Regenerating them
  requires a local Edax checkout (`eval.dat`, matching the `EDAX_HOST_DIR`
  env var used elsewhere in this repo — see `.env.sample`):
  ```
  cargo run --manifest-path wasm/edax-eval/Cargo.toml --bin extract_weights --release -- wasm/edax-eval/generated
  cargo build --manifest-path wasm/edax-eval/Cargo.toml --target wasm32-unknown-unknown --lib --release
  cp wasm/edax-eval/generated/weights.bin.gz wasm/edax-eval/generated/weights_manifest.json wasm/edax-eval/dist/
  cp wasm/edax-eval/target/wasm32-unknown-unknown/release/edax_eval.wasm wasm/edax-eval/dist/
  ```
  Only needs re-running if `wasm/edax-eval`'s Rust source changes (rebuild
  `edax_eval.wasm`) or `eval.dat` itself changes (regenerate
  `weights.bin.gz`) — CI can't do this itself (no `EDAX_PATH`/`eval.dat`
  there), so it's a manual step before committing, same as any other
  `EDAX_PATH`-gated local-only workflow in this repo. Forgetting the
  `edax_eval.wasm` half now fails loudly:
  `wasm/edax-eval/js/dist-freshness.test.js` (run by `test.sh` and CI)
  compares the committed artifact against a fresh build. Regenerating
  `weights.bin.gz` is *not* covered — nothing in CI has `eval.dat` to
  compare against.
