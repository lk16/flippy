# Flippy: search depth parity, stats page, queue tiers, bug fixes

Self-contained work order. Read `CLAUDE.md` and the docs it links first —
especially `docs/style.md` (comments: one line per exported symbol plus
constraints the code can't express; no design essays) and
`docs/testing.md`.

## How to work

- One commit per change below, in the order given. Later changes depend on
  earlier ones.
- Before each commit: `pre-commit run -a` and `./test.sh`. Both must pass.
- Backend code outside `cmd/*/main.go` needs tests. If a change leaves
  lines uncovered, write tests — never add coverage-ignore tooling.
- `internal/edax` subprocess tests need a real edax binary (`EDAX_PATH`);
  they skip themselves without one, as in CI.
- Do not add CHECK constraints to `boards`. Validation lives in the app
  layer, by design.

---

## 1. Depth parity: stop the evaluation graph zig-zagging

### The problem

In the PGN evaluation graph, scores alternate between adjacent values
(0, 2, 0, 2, …) along a line of best moves. That is search bias, not
signal.

Every target search on a position that is *depth-limited* (depth < empties,
so the search stops before the end of the game) uses an even depth: the
tiers in `internal/api/jobs.go` are levels 40/36/34/32 and in that regime
`edax.SearchParams` returns depth == level. An even-depth search ends with
the *opponent* making the last move, which scores the side to move about
0.8 discs low; an odd-depth search scores it about 0.8 discs high. Scores
are stored mover-relative, and the mover swaps colour every ply, so a graph
drawn in black's point of view alternates by ~1.6 discs — which on integer
scores reads as 0, 2, 0, 2.

### The evidence

Measured over the whole book: for each position at its target level whose
children are all at target, `parent score + best child score` should be 0.
It is not.

```
disc  lvl depth  conf horizon    pairs  mean gap  median
  12   40    40   73%      52     1435    -1.890    -2.0
  16   36    36   73%      52     3383    -1.963    -2.0
  20   34    34   73%      54     6051    -1.819    -2.0
  23   32    32   73%      55     7364    -1.641    -2.0
  24   32    32   73%      56     7664    -0.263     0.0
  25   32    39   73%      64     7925    -0.016     0.0
  27   32    37   73%      64     8302     0.180     0.0
  29   32    35   95%      64     8573    -0.010     0.0
```

Exactly −2 wherever the search is depth-limited; ~0 from 25 discs up,
where the search runs to the end of the game so depth == empties and its
parity flips every ply on its own.

Confirmed against the real binary on 300 randomly sampled book positions
per disc count, comparing four depth schemes (depths 12 and 13):

```
disc  parents |  all-even |  all-odd |  matched |     anti
 all          |    -1.557 |   +1.587 |   -0.017 |   +0.047
```

`matched` = depth parity follows disc-count parity, `anti` = the opposite.
Both alternating schemes work; the constant schemes do not. So what matters
is only that adjacent plies use *different* depth parities.

### The fix

Raise the level by one when the search stops before the end of the game and
its depth parity does not match the disc count's:

```go
func aligned(discCount, level int) int {
	depth, _ := edax.SearchParams(discCount, level)
	if depth < 64-discCount && depth%2 != discCount%2 {
		return level + 1
	}
	return level
}
```

Applied to the current tiers this touches six disc counts and nothing else.
Confidence stays 73 % throughout, so no selectivity changes:

```
discs  base -> new   depth
   13    40 -> 41    41@73%
   15    36 -> 37    37@73%
   17    34 -> 35    35@73%
   19    34 -> 35    35@73%
   21    32 -> 33    33@73%
   23    32 -> 33    33@73%
 12,14,16,18,20,22,24  unchanged
 25+                   unchanged (depth == empties already)
```

Raise, don't lower: `SaveEvaluation` only accepts a higher level, so
lowering odd targets would strand the existing biased rows above target and
they would never be redone.

Apply the rule to **every** level that ends up as a search level, not just
the tier table — including the interactive level from change 6. Once a
level is aligned, climbing it by 2 keeps it aligned.

### Where

- `internal/api/jobs.go` — `TargetLevel`, `EffectiveTargetLevel`. Note
  `EffectiveTargetLevel` clamps the disc count to `book.MaxSavableDiscs`
  before the tier lookup; the parity rule must use the position's **real**
  disc count, not the clamped one.
- `internal/api/handlers.go` — `levelConfigResponse` must carry enough for
  the frontend to reproduce the rule (the tier table alone no longer
  determines the target).
- `static/board.js` — `targetLevelForBoard`, the hardcoded fallback
  `levelConfig`, and `LOCAL_EVAL_LEVELS` / `localEvalLevelsFor` (the WASM
  ladder is all-even too, so browser-side scores zig-zag the same way).
  `localEvalLevelsFor`'s rung thresholds are level-dependent and need
  adjusting alongside.
- `static/test/harness.js` — `DEFAULT_LEVEL_CONFIG` mirrors the API.
- `internal/api/jobs_test.go` — `TestTargetLevelTiers_MatchTargetLevel`
  guards a contract that changes.

### Scope

Explicitly **not** fixed: alternating the parity removes the zig-zag but
leaves a constant offset of about −0.8 discs in black's point of view, so
the minimaxed tree root keeps reading −2 rather than the true 0. Removing
that would need each position searched at two adjacent depths and the two
averaged (~2.3× the one-off search bill, plus half-disc storage). Decided
against — a constant offset is not noise, and picking the opposite parity
because it makes the root read 0 would be fitting the answer.

### Cost and rollout

~523k rows currently at target (disc counts 13/15/17/19/21/23) drop below
target and get re-searched: 13 → 21k, 15 → 33k, 17 → 65k, 19 → 100k,
21 → 137k, 23 → 167k. Rows still below target were going to be searched
anyway, one ply deeper. The extra ply is not extra cost — measured over the
same position set, the odd level ran 15–19 % *faster* than the even one.
The graph will show a partial zig-zag while the book converges.

### Acceptance test

Re-run the parent-vs-best-child measurement: for each disc count, take
positions at target level whose children are all at target, and average
`parent score + min(child scores)`. Every disc count should read ~0, the
way 25+ discs already does.

---

## 2. Stats page: two header rows, far fewer columns

`static/stats.js` builds one column per distinct `(depth, confidence)` pair
across all disc counts, plus `unlearned` and a `max @ x%` column per
confidence. That is dozens of columns.

Replace with two header rows:

- **Row 1 — groups:** `Unlearned` | `Partially learned` | `Learned`
- **Row 2 — columns:**
  - under Unlearned: one column, `0`
  - under Partially learned: one column per search *below* that row's
    target, labelled as today (`16 @ 73%`, …, `32 @ 73%`)
  - under Learned: two columns, `target` and `count`. `target` shows the
    target for that row's disc count; `count` is how many positions at that
    disc count are fully learned.

Keep the per-row and per-column totals.

"Learned" means the row's evaluation reaches the target search or better —
i.e. `edax.IsFinal(discCount, level)`, or `(depth, confidence)` equals
`edax.SearchParams(discCount, TargetLevel(discCount))`.

`/api/stats` returns `(disc_count, depth, confidence, count)` entries and
the `book_stats` Redis hash is keyed `<depth>:<confidence>:<discs>`, so the
level is not available downstream — but the bucket is still derivable from
`(disc_count, depth, confidence)` alone. Do that classification
server-side in `internal/api/handlers.go` (`statEntries`) and in
`getBookStats`, and add the bucket plus the row's target to the JSON, so
`stats.js` does not need a JS port of `edax.SearchParams`. No change to the
hash layout.

Touches: `internal/api/handlers.go`, `internal/api/redis.go`
(`getBookStats`), `static/stats.js`, `static/stats.css`, and the stats JS
tests under `static/test/`.

---

## 3. Bug: rows below the minimax leaf disc count

On prod, positions with 5 and 6 discs turned up in the DB (or at least in
the frontend). Nothing below `book.LeafDiscs` (12) should ever get a row —
everything under it is minimax-derived by `internal/book`.

Root cause: `handleJobResult` in `internal/api/handlers.go` guards the
priority insert paths with `discCount <= book.MaxSavableDiscs` only — there
is no lower bound. So any frontend analysis request for a position below 12
discs that a worker answers gets a row inserted (both the
`!isBookQuality(...)` branch and the `isPriority` branch). The loader path
is already guarded: `loader.isSavable` requires
`discs >= book.LeafDiscs && discs <= book.MaxSavableDiscs`.

Fix:

- Put the range check in one place that no caller can bypass —
  `db.Repository.AddPositionsInserted` filtering or rejecting out-of-range
  positions — and keep the caller-side guards in `handleJobResult`.
- No DB CHECK constraint (see "How to work").
- Add a one-off cleanup to `scripts/` deleting rows with
  `disc_count < 12 OR disc_count > 30`, for prod.
- Also confirm the frontend cannot *display* such a position as a book
  entry: check what `lookupEvaluation` returns below 12 discs and that the
  minimax cache is the only source there.

Verify against prod with `SELECT disc_count, count(*) FROM boards WHERE
disc_count < 12 GROUP BY 1;` before and after.

---

## 4. Bug: PGN search status line is glitchy

`pgnUpdateGraphStatus` in `static/board.js` reports
`Searching at level N — done / total boards evaluated…`, where N is the
minimum over `pendingLevelRequests` for still-unresolved positions. It
jumps around because positions ramp their level in +2 rounds
(`pgnRequestLevelUps`) and drop out of the set as they resolve.

With change 6 there is a single interactive level, so the ladder is gone.
Delete `pgnRequestLevelUps` and `pendingLevelRequests` along with it, and
reduce the status line to progress only — `N / M positions evaluated`, and
a settled state when done. Do this in the same commit as change 6 if that
is cleaner than splitting it.

---

## 5. Bug: grandchildren evaluation requests flood the websocket

`requestGrandchildrenEvaluations` in `static/board.js` sends every on-book
grandchild to the server via `requestServerAnalysis`. There is roughly an
order of magnitude more of them than children, and none is on screen — the
resulting `analyze_request` payloads are enormous.

Only direct children go to the server. Keep the local WASM prefetch for
off-book grandchildren (the server is not an option for those anyway).

`hasUnresolvedEvaluations` also walks grandchildren to decide whether to
keep polling — it must stop counting them, or polling will never settle
once the server stops answering for them.

---

## 6. Redis queues: three priority tiers

Minimal changes to `internal/api/redis.go`, `internal/api/jobs.go` and
`internal/db/repository.go` (`ListLearnable`). Tiers, highest first:

1. **Requested from the frontend** → search at level 16.
2. **In book but unlearned** (`level = 0`) → search at level 16.
3. **In book but below target** → search at target.

All three levels go through change 1's parity alignment, so tier 1 and 2
are 16 at even disc counts and 17 at odd ones.

Today: one priority list (`priority_jobs`, level from the request, default
`PriorityLevel = 10`), then one shared buffer (`job:buffer`) fed by
`ListLearnable` ordered by disc count, then level, then position. That
ordering puts unlearned rows first only *within* a disc count, so a
12-disc partially-learned row still outranks a 20-disc unlearned one.

Changes:

- `PriorityLevel` becomes 16 (aligned), and the request level is no longer
  a ladder — drop the frontend's climb-by-2 (see change 4).
- Make tier 2 global: order the learnable scan by "unlearned first", then
  disc count, then level, then position. Keep `LearnableCursor` encoding
  and `decodeJobCursor` in step with the new ordering.
- `GetJob` hands out level 16 for an unlearned row and the target level for
  a below-target row, instead of always the target.

A row learned at 16 then falls into tier 3 and is re-searched at target.
That is intended. The book already holds ~800k level-16 rows at several
disc counts, so tier 2 mostly formalises what the data already looks like.

### Disconnect promotion

`dequeuePriority` currently discards an entry whose requesting connection
has closed (`if entry.ConnID != "" && !s.connLive(...) { continue }`) —
the work is thrown away. Instead, promote it: add the position to the book
as an unlearned row (level 0, score 0) so it becomes tier 2, then continue.
Respect change 3's disc-count range — only positions in
`[book.LeafDiscs, book.MaxSavableDiscs]` can be added; drop the rest as
today.

---

## 7. Logging

Concise, human-readable, one line each. No log spam in hot paths.

**Server** — surface the numbers that make later benchmarking possible,
especially values read from Redis:

- job buffer refills: how many candidates buffered, and where the cursor is
- priority queue depth, and how many entries were dropped or promoted on
  dequeue
- `book_stats` resync: duration and row count
- the computed job floor
- claim attempts: taken vs. already-claimed

**Worker** — `logStats` in `internal/worker/worker.go` currently prints
`N positions done, X positions/sec, Y sec/position`. Print positions/hour
with one decimal instead, and drop the other two rates.
