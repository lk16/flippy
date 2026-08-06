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

## Derive depth and confidence from disc count + level

Not started. Research below; the numbers come from replaying the ported
`depth_and_selectivity` over every (level, empties) pair.

### The relation

Edax's `search_global_init` (`search.c:161-346`) maps `(level, n_empties)`
to `(depth, selectivity)`, and `selectivity_table` (`search.c:104-111`)
maps the selectivity index to a percentage — the `{73,87,95,98,99,100}`
enum. Both are ported verbatim in
`wasm/edax-eval/src/search.rs` (`depth_and_selectivity`,
`SELECTIVITY_TABLE`), with a spot-check test against the C table.

`n_empties = 64 - disc_count`, so **`(disc_count, level)` determines
`(depth, confidence)` exactly**. Confirmed against a real edax run:
`internal/edax/parser_test.go`'s fixture is the 60-empty start position at
level 20 and edax prints `20@73%`, which is
`depth_and_selectivity(20, 60)`.

Two things that look obvious and aren't:

- **100% confidence does not mean solved.** It means `NO_SELECTIVITY` — no
  forward pruning. A level-4 search of a 24-disc board reports `4@100%`.
  The "nothing left to learn" predicate is
  `depth == 64 - disc_count && confidence == 100`, i.e. the search reached
  the end of the game full-width. Only then is the score final.
- **`(depth, confidence)` is not monotone in level.** At 25+ empties,
  level 10 gives `10@100%` and level 11 gives `11@73%`. Level orders
  results; confidence on its own does not.

### Drop the two columns

At its target level every board in the book comes out at 73% confidence,
with `depth = min(level, n_empties)` — six distinct `(depth, confidence)`
pairs across the whole table:

| discs | target level | stored |
|---|---|---|
| 12-16 | 32 | `32@73%` |
| 17-20 | 30 | `30@73%` |
| 21-27 | 28 | `28@73%` |
| 28-30 | 28 | `36@73%`, `35@73%`, `34@73%` (exact depth) |

Nothing in SQL reads either column except `SaveEvaluation`'s
`($1, $3) > (level, confidence)` guard (`internal/db/repository.go:114`),
and that half is already dead: same board + same level ⇒ same confidence,
so the tuple compare reduces to `$1 > level`. Everything else just
`SELECT`s them through (`GetBoard`, `ListLearnable`, `EvaluatedBoards`).

So: a migration dropping `depth` and `confidence`, plus a Go
`edax.SearchParams(discCount, level) (depth, confidence int)` — the same
port as the Rust one, tested against the same table — computed on read in
`scanBoardEvaluations`. A lookup table + FK works too, but the key would
be `(disc_count, level)` and `boards` already stores both, so it buys a
constraint and no space; prefer it only if SQL ever needs to filter on
depth or confidence. `jobResultRequest.Depth/Confidence`
(`internal/api/types.go`) become derivable the same way — worth keeping on
the wire for one release and checking against the formula in
`validateJobResult` before removing.

### Skip searches that cannot tell us anything new

The book itself is already tight: over disc counts 12-30, no two levels
below the target produce the same `(depth, confidence)`, and an exact
solve at 34 empties needs level 40 — far above the target of 28. So every
`+2` rung there is real work.

The waste is in the endgame, and PGN review walks straight into it:
`pgnSendRequests` (`static/board.js`) sends the whole line to the server
without `splitOffBook`, so a 44-disc board gets analyzed at levels 10, 12,
… 28 — ten jobs, all returning the identical `20@100%` exact solve,
because at 20 empties every level from 10 up solves the game outright.

| discs | empties | levels 4 → 28 |
|---|---|---|
| 32 | 32 | `4@100%` … `10@100%`, `12@73%` … `20@73%`, `32@73%`, `32@95%` |
| 40 | 24 | `4@100%` … `10@100%`, `24@98%`, `24@100%` from level 20 up |
| 44 | 20 | `4@100%`, `6@100%`, `8@100%`, then `20@100%` from level 10 up |
| 56 | 8 | `8@100%` at every level from 4 up |

Three fixes fall out:

- `handleAnalyzeRequest` (`internal/api/websocket.go`) should skip a board
  whose stored evaluation is already final
  (`depth == n_empties && confidence == 100`) — no level can improve it —
  and otherwise clamp the requested level down to the lowest one yielding
  the same `(depth, confidence)`, collapsing distinct levels that mean the
  same search.
- `isAtTarget` (`static/board.js`) should count a final result as at
  target whatever its level, so `pgnRequestLevelUps` stops climbing. The
  client-side counterpart already landed: `localEvalLevelsFor` picks the
  wasm ladder's rungs from the same relation.
- The priority queue dedups by board string only
  (`internal/api/priority.go:41-54`), so two requests at different levels
  collapse to whichever arrived first. Keying on the resulting
  `(depth, confidence)` instead would also merge different-level requests
  that describe the same search.

### Also worth a look

- `validateJobResult` (`internal/api/handlers.go:81`) range-checks depth
  and confidence independently; it could assert the exact expected pair.
- `TargetLevel`'s tiers are expressed in level, which means very different
  work per board: at level 28 a 21-disc board gets a 28-ply midgame search
  and a 30-disc board a full 34-empty solve. If the intent is roughly
  equal cost per board, tier on the resulting `(depth, confidence)`.
- `book validate` (listed under CLI tools above) gets a real check:
  recompute `(depth, confidence)` from `(disc_count, level)` and flag rows
  that disagree.

Risk to weigh first: the relation is edax-4.5.1-specific. An upgrade that
retunes `search_global_init` silently reinterprets every stored row, and
dropping the columns removes the evidence. Stored `depth` is also edax's
*reported* depth from its last printed line, so a search cut short would
read shallower than the formula — nothing cuts one short today, but the
validation should warn before it rejects.

## Testing gaps

- The websocket client's reconnect/queueing logic has no JS unit tests;
  `static/test/` covers board logic only.
- No browser-level regression tests at all: nothing catches a frontend
  wiring bug (e.g. evaluations not appearing under the legal moves) short
  of opening the page by hand. Playwright is the intended answer;
  [playwright-e2e-prep.md](playwright-e2e-prep.md) lists what the host has
  to prepare first, since the sandbox can't reach npm or Playwright's CDN.

## Frontend

- **PGN review doesn't use the local wasm evaluator.** Normal play falls
  back to `queueLocalEvaluations` for every child the server hasn't
  answered for, so a score appears under each move right away; PGN review
  still shows blanks until the server answers. That wait covers the whole
  line: unlike normal play, `pgnSendRequests` skips `splitOffBook` and
  sends boards past `MaxSavableDiscs` (30) to the server too, which does
  evaluate them — into the ephemeral cache, never the DB.
  Wiring it up is not just a call to `queueLocalEvaluations`: the same
  evaluations feed the score graph, `pgnUnresolved`/`isAtTarget` and the
  level-up chain, all of which treat an evaluation as the server's answer.

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
