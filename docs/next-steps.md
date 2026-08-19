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
  level/score bounds, but nothing rejects a too-shallow one — it is the
  DB write that is gated on the board's target level (`isBookQuality`),
  not the request. Depth and confidence are checked against the level
  table but only warned about (`checkReportedSearchParams`), never
  rejected.
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

## Depth and confidence: what is left

Landed: `edax.SearchParams` derives both from `(disc_count, level)`,
migration 000002 dropped the columns, the stats page groups by the
resulting search, and `handleAnalyzeRequest` skips a level that means a
search the board already got. What is still open:

- The priority queue dedups by board string only
  (`internal/api/priority.go`), so two requests at different levels
  collapse to whichever arrived first. Keying on the resulting
  `(depth, confidence)` instead would also merge different-level requests
  that describe the same search.
- `TargetLevel`'s tiers are expressed in level, which means very different
  work per board: at level 32 a 21-disc board gets a 32-ply midgame search
  and a 30-disc board a full 34-empty solve. If the intent is roughly
  equal cost per board, tier on the resulting `(depth, confidence)`.
- `book validate` (listed under CLI tools above) gets a real check:
  recompute `(depth, confidence)` from `(disc_count, level)` and flag rows
  that disagree — for a book, now, only in the sense of flagging a level
  the current edax would answer differently.

Two things that look obvious and aren't, worth re-reading before touching
any of this:

- **100% confidence does not mean solved.** It means `NO_SELECTIVITY` — no
  forward pruning. A level-4 search of a 24-disc board reports `4@100%`.
  The "nothing left to learn" predicate is `edax.IsFinal`:
  `depth == 64 - disc_count && confidence == 100`.
- **`(depth, confidence)` is not monotone in level.** At 25+ empties,
  level 10 gives `10@100%` and level 11 gives `11@73%`. Level orders
  results; confidence on its own does not.

Standing risk: the relation is edax-4.5.1-specific. An upgrade that
retunes `search_global_init` reinterprets every stored row, and the
columns that were the evidence are gone. What is left is the warning in
`checkReportedSearchParams` (a worker's reported pair vs. the formula)
and `scripts/verify_derived_columns.sql` for a book still carrying the
old columns.

## Import the pre-rewrite book archive

Not started. Numbers below come from `old/ignored/edax_1763247601.sql.gz`
(2,422,020 rows, Nov 2025) against `backups/boards_1786462307.sql.gz`
(14,121,197 rows) — both gitignored, so re-derive if they move.

### All of it transfers, not half

The old `edax` table stored one row per position with the turn thrown
away; `boards` stores the turn. That looks like only the parity-matching
half can be reused. It isn't a constraint at all: **the score depends only
on `(mover, opponent)`, never on which colour holds which discs.** Three
independent confirmations:

- **Edax's own representation.** `Board` is `{player, opponent}`
  (`bit.h:147`). `board_set` (`board.c:101-146`) reads `X` into `player`
  and `O` into `opponent`, then calls `board_swap_players` if the turn
  field says `O`. Colour is discarded before the search starts, so a
  colour-swapped problem line with the flipped turn is bit-identical
  input.
- **Measured.** 500 book positions solved at level 14 with `-n-tasks 1`,
  each as itself and colour-flipped: 500/500 identical scores (67 distinct
  values). At `-n-tasks 4`, 7/500 differ — but the same file against
  itself differs in 5/500, so that is parallel-search nondeterminism, not
  the flip.
- **In the data.** On the 26,173 covered rows where both books searched at
  the same `(level, depth, confidence)`, scores agree 94.05% for
  black-to-move and 93.84% for white-to-move. No colour asymmetry; the 6%
  gap is that same nondeterminism (77% of it is ±1).

### Mapping the rows

Normalization is unchanged — both minimize `(mover, opponent)` over the 8
symmetries, with identical flip primitives. Against that one shared rule,
0 of 2,422,020 old rows and 0 of 14,121,197 new rows come out
unnormalized, and every row's `disc_count` matches its bitboards. Only the
encoding differs:
old is 16 bytes little-endian `(player, opponent)`, new is 17 bytes
big-endian `(black, white, turn)`.

Turn follows disc parity — even is black to move, odd is white — with
passes as the only exception, and they are rare (at 30 discs: 1,450,922
black-to-move rows against 1,028 white). So map an old row to
`turn = parity`. Writing *both* turns is also always sound and picks up
the pass positions, at the cost of ~2.4M rows that only a pass line will
ever look up — dead weight in the row counts, not wrong answers.

### What it buys

Old rows answer 2,366,277 new rows (16.8% of the book), split 1,261,106
black-to-move and 1,105,171 white-to-move — both parities land, as
predicted. Only 55,743 old positions are missing from the book entirely.
The archive is never the weaker of the two: `old.level < new.level` in 0
of those 2.37M rows (the book is mostly level 16, the archive 32-40).

`TargetLevel`'s tiers were then set to the archive's own maximum at each
disc count (40/36/34/32), so an imported row lands exactly at target
rather than above or below it. Counting rows at or above `TargetLevel`:

| | at target |
|---|---|
| now | 0 |
| after import | 2,339,191 (16.6%) |

That is **2,339,191 target-level searches avoided**, and the expensive
kind — level 32-40 against a book currently sitting mostly at 16. Fairly
flat across disc counts: every 12-disc board, then ~22% of the rest at
13-16 discs sliding to 14.5% at 30.

Nothing is at target beforehand because the tiers moved: the 206,533 rows
that met the old 32/30/28 targets no longer meet 40/36/34/32. That is the
cost side of matching the archive — the 11.8M rows it does *not* cover now
need a deeper search than they did before.

### Import notes

- Drop the old `depth`/`confidence` columns on import: `boards` no longer
  has them (they follow from `(disc_count, level)`, exactly — 0 of the
  14,121,197 rows the book held at the time disagreed).
- 949 old rows (0.04%) carry a `depth` below what their level implies,
  usually 0 — exactly the cut-short searches the previous section warns
  about. Their `level` overstates the search that produced the score, so
  drop them instead of trusting either column.
- `best_moves` has no counterpart in `boards`; it is dropped on import.
- Load into a staging table and let the existing `$1 > level` guard in
  `SaveEvaluation` decide, so a row the book already has at a higher level
  is not downgraded.

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
