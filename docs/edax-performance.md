# Edax performance at high levels

Research notes on making the worker evaluate more boards per hour at target
levels and above (32-40). Nothing here is implemented; every number was
measured, the source references are from the real Edax source.

**Bottom line.** Cost per board multiplies by **2.13 per +2 levels**, so
32 -> 40 is ~21x. Every engine-level knob we found is worth at most ~6%
combined, and most "obvious" tricks (bigger hash, aspiration windows, hash
seeding, cross-position transposition reuse, multi-threading) measured at
zero or negative. Level choice and worker count are the only large levers.

## Setup

- Host: sbx sandbox VM, 4 vCPU AMD Zen 2 (family 23 model 113), AVX2+BMI2,
  no AVX-512, 16 MB L3. Absolute numbers are machine-specific; ratios are
  the useful part.
- Engine: `~/projects/INACTIEF/edax-reversi` = lk16/edax-reversi, Edax
  4.5.3 (Okuhara AVX fork), stock `bin/lEdax-x64`. ~19.5 Mnode/s
  single-threaded.
- Positions: random samples of `internal/othello/precomputed_boards_12discs.txt`
  (12 discs / 52 empties), 8-board and 4-board sets, converted to `.obf`.
- All runs `-n-tasks 1` unless stated. Node counts are deterministic and are
  the primary metric where wall clock was noisy.

## Cost model

`-solve`, 1 thread, 12-disc boards. Two board sets, so compare within a
column:

| level | s/board (8-board set) | s/board (4-board set) |
|------:|---------------------:|---------------------:|
| 24 | 0.38 | |
| 26 | 0.62 | |
| 28 | 1.51 | |
| 30 | 5.60 | |
| 32 | 8.59 | 5.2 |
| 34 | 16.8 | 11.1 |
| 36 | | 22.2 |
| 38 | | 51.0 |
| 40 | | ~110 (extrapolated) |

Geometric growth per +2 levels: 2.13x (8-board set, 24->34) and 2.14x
(4-board set, 32->38). So per +1 level ~1.46x, and 32 -> 40 ~= 21x.
Node rate stays flat at 19.0-19.7 Mnode/s across all levels: the cost is
tree size, not slower nodes.

For 44-52 empties, `search_global_init` (`src/search.c:161-346`) maps
level 28-40 to `depth = level, selectivity = 0` — i.e. a plain midgame
search at the *most* aggressive ProbCut setting (73%). There is no
selectivity left to trade away.

## 1. Compile flags: ~6%, from `-march=native` only

Rebuilt Edax from the same source with gcc 14 in Docker. Identical node
counts (232,939,049) across all builds, so this is pure speed:

| build | Mnode/s | vs stock |
|---|---:|---:|
| stock `bin/lEdax-x64` | 19.62 | 1.000x |
| `make build ARCH=x64` | 19.75 | 1.009x |
| `make build ARCH=x64-modern` (`-march=core-avx2`) | 18.70 | **0.956x** |
| `-march=native` (znver2) | 20.85 | **1.059x** |
| `-march=native` + PGO | 20.42 | 1.041x |
| `ARCH=x64-avx512` binary | — | SIGILL (no AVX-512 here) |

- `x64-modern` is *slower* than generic `x64` on Zen 2. `settings.h` only
  swaps `MOVE_GENERATOR_SSE` (34.4 Mnps per Edax's own comment) for
  `MOVE_GENERATOR_AVX` (34.7 Mnps) — under 1% — while `-march=core-avx2`
  applies Haswell scheduling, which costs more than that.
- `-march=native` wins because of znver2 tuning, not AVX2.
- PGO (`make pgo-build`, or `-fprofile-generate/-use`) trained on 8 book
  positions at level 22 gave no gain over plain native. Untested with a
  larger/representative training set.
- `COUNT_LAST_FLIP_BMI2` is marked "slow on AMD" in `settings.h`; it is not
  selected by any of the gcc ARCH targets, so no action needed.
- On a worker with AVX-512, `ARCH=x64-avx512` also switches to
  `MOVE_GENERATOR_AVX512` + `COUNT_LAST_FLIP_AVX512`. Untested — worth
  measuring if such a host exists.

**Action:** build per-host with `-march=native`:
`gcc -std=c99 -pipe -D_GNU_SOURCE=1 -DUNICODE -Ofast -fwhole-program -DNDEBUG
-flto -m64 -march=native -DUSE_GAS_X64 -DPOPCOUNT -DLASTFLIP_HIGHCUT all.c -s
-o lEdax-native -lm` (run from `src/`). Skip `x64-modern`, skip
PGO.

## 2. Runtime flags: nothing left on the table

- **`-hash-table-size`** (default 22 = 2^22 entries, 113 MB). Node count
  saturates well below the default. Level 30: `-h 14` +11% nodes, `-h 18`
  / `22` / `25` within 0.4%. Level 34: `-h 18` +3.3%, `-h 20` +0.6%,
  `-h 22`/`24` flat. Bigger is never faster.
  **Action (memory only):** `-h 20` costs <1% and saves ~85 MB per worker.
- **`-probcut-d`** (default 0.25) is at its optimum. Level 30, 4 boards:
  0.20 -> 2.4x slower, 0.35 -> 2.8x slower, 0.50 -> >14x slower (aborted).
- **`-inc-{pv,cut,all}node-sort-depth`** (default `0/-2/-3`): all variants
  tried were within ±2%.
- **`-selectivity`** cannot help: level 28-40 at these empty counts is
  already selectivity 0 (73%), the most aggressive entry in
  `selectivity_table` (`src/search.c:104-111`).
- **`-n-tasks`**: see §4.

## 3. I/O and worker/server interaction: not where the time goes

Measured fixed costs, against 8.6 s/board (level 32) or 51 s/board (level 38):

| cost | measured |
|---|---|
| edax process start (eval.dat load + hash alloc) | 48 ms (`-h 18`), 78 ms (`-h 22`), 179 ms (`-h 24`) |
| hash wipe per problem inside a running process | 0.3 / 4.2 / 18.3 ms at `-h 18` / `22` / `24` |
| stdout at `-verbose 3` | ~2 KB per board (~230 B/s at level 32) |
| result submission | one blocking HTTP POST per board |

Together well under 0.2% of a level-32 job. Job fetching is already batched
and prefetched (`internal/worker/worker.go`). There is no I/O win available.

Two real (small) defects:

- `edax.Process` restarts the subprocess whenever the requested level
  changes (`ensureStarted`, `internal/edax/process.go`). Priority-queue jobs run at
  level 10, 12, ... (`api.PriorityLevel`) and interleave with book jobs at
  level 28-32, so each switch pays ~78 ms — small for a book job, but
  larger than the ~10 ms search itself for a level-10 job. Fix: one process
  per level class, or Cassio mode (§5), where depth is per request.
- `-verbose 1` already prints everything `edax.Evaluation` needs on one line
  (`  1|24@73%  +00  0:00.207  3948750  19076087 g4 G5 ...`) instead of the
  full table `-verbose 3` produces. No measurable speed gain; simpler parse.

## 4. Threads vs processes: current setup is already optimal

Each process solves the *same* 8 boards (equal work per process, so no
chunking artefact), level 28, 4 vCPU:

| config | boards/hour | vs 1x1 |
|---|---:|---:|
| 1 proc x 1 task | 2,333 | 1.00x |
| **4 proc x 1 task** | **8,815** | **3.78x** |
| 2 proc x 2 tasks | 7,277 | 3.12x |
| 1 proc x 4 tasks | 4,392 | 1.88x |
| 2 proc x 4 tasks | 4,925 | 2.11x |
| 8 proc x 1 task | 8,588 | 3.68x |

`EDAX_TASKS=1` with one worker per core is right. Edax's YBWC parallel
search buys latency, not throughput, and oversubscribing (8 procs on 4
cores) buys nothing.

Caution for anyone re-running this: an earlier version statically split the
board list across processes and made `1 proc x 4 tasks` look 1.8x *better*.
That was load imbalance between chunks, not a real effect. Give every
process identical work, or use a shared queue.

## 5. Cassio engine protocol: a better driver, but no free speed

`edax -cassio` (`src/main.c:141`, `src/cassio.c:engine_loop`) speaks a line
protocol on stdin:

```
ENGINE-PROTOCOL init | new-position | empty-hash | quit
ENGINE-PROTOCOL midgame-search <board> <alpha> <beta> <depth> <precision>
ENGINE-PROTOCOL endgame-search <board> <alpha> <beta> <precision>
ENGINE-PROTOCOL feed-hash <board> <lower> <upper> <depth> <precision> <pv>
```

One reply line plus `ready.`:

```
<board>, move G4, depth 20, @73%, B-1.00 <= v <= B-1.00, G4g5E2c4..., node 1143322, time 0.064
```

Verified equivalent to `-solve -level L` on 12-disc boards: node counts
within 0.2%, same score/depth/selectivity (level 24: solve 3,948,750 nodes
`+00 @73%` vs cassio 3,955,486 nodes `0 @73%`).

Why it is attractive: depth and precision are per request, so no process
restarts on level change; output is one machine-readable line; the
transposition table survives across positions.

Gotchas in `engine_open` (`src/cassio.c`), all under the default
`transgress_cassio = true`:

- requested depth is bumped by 1 if its parity differs from `n_empties`;
- depth > `n_empties - 10` is promoted to a full exact solve;
- precision is forced to 73% whenever `depth < n_empties`.

So flippy would have to reproduce `search_global_init`'s LEVEL table to keep
the stored `level` column meaningful. `-follow-cassio` disables the
transgressions.

Measured value of the extra capabilities — mostly negative:

- **Transposition reuse across positions: none.** 6 sibling boards (13
  discs, same parent) at depth 26: 93.1M nodes with the hash emptied
  between, 93.1M with it kept (100.0%). 6 unrelated 12-disc boards: also
  100.0%. Control: repeating the *same* board in a warm process returns in
  **0 nodes**, so the table really is live — sibling search trees just do
  not overlap at these depths. Ordering jobs by tree locality would buy
  nothing.
- **Aspiration windows (`-alpha`/`-beta`): none.** Handing Edax the true
  score ±2 changed nodes by 0.1%; ±1 saved 15% but returned a bound instead
  of an exact score in 2 of 4 boards. `iterative_deepening` already runs its
  own aspiration windows.
- **`feed-hash` seeding: harmful.** Feeding a board's own depth-26 result
  (bounds + PV) then searching depth 30 cost 89.3M nodes vs 26.7M cold
  (3.3x worse). Re-searching in a warm process was worse still (99.9M): a
  repeated board makes `is_position_new` false, so Edax takes
  `aspiration_search` at full depth instead of `iterative_deepening`, losing
  the move ordering that iterative deepening builds up.
- **The one win: incremental deepening on a single board.** Depths
  10,12,...,28 cost 34.2M nodes as 10 separate cold searches, but 18.0M in
  one warm process — exactly what a single cold depth-28 search costs
  (18.0M). That is precisely the frontend's priority-queue ladder
  (`api.PriorityLevel` then +2 per round), so serving it from one persistent
  Cassio process would roughly halve its cost.

Note: at selectivity 0 the score is not a pure function of (position,
depth) — it depends on hash state and move ordering. Two runs of the same
board at the same level can differ by a disc (observed: 0 vs -1).

## 6. A Rust engine

`wasm/edax-eval` on `origin/edax-rust-wasm` (~4.4k lines) already ports
board/movegen, incremental feature eval + `search_eval_0`, the LEVEL
depth/selectivity table, `eval_sigma`, `selectivity_table`, ProbCut,
PVS/NWS with a 2^14-entry direct-mapped TT, endgame stability cutoffs and
mobility move ordering. It is bit-exact against real Edax for level <= 10.

Shipped `dist/edax_eval.wasm` vs Edax, same 2 twelve-disc boards, 1 thread:

| level | wasm port | edax | ratio |
|---:|---:|---:|---:|
| 10 | 0.38 s | 0.112 s | 3.4x |
| 12 | 0.32 s | 0.022 s | 14.5x |
| 14 | 1.16 s | 0.032 s | 36x |

(Level 12 is cheaper than level 10 for both, because level 10 at 52 empties
is full-width while level 12 already uses ProbCut.)

The gap widens with depth because what is missing is exactly what pays off
deeper: a large multi-way hash (Edax: 2^22 entries, 4-way, vs 2^14
direct-mapped), enhanced transposition cutoff, PV extension, Edax's full
move-ordering evaluation, the endgame-specialised searches
(`DEPTH_TO_SHALLOW_SEARCH`, `DEPTH_MIDGAME_TO_ENDGAME`, the
`count_last_flip_*` variants), SIMD flip, and YBWC parallel search. A native
(non-wasm) build with a real TT would recover part of it, but at level 32-40
the port is currently >100x off and the target is a 25-year-tuned engine.
The realistic best case for finishing it is parity, not a speedup. Scores
also already diverge above level 10 (level 12: port 0, Edax +1), so it would
need its own accuracy story.

The one thing a custom engine could do that Edax cannot: **use the book as a
leaf oracle** — cut the search wherever the position is already in `boards`
at sufficient level. Edax's only hook for that is `feed-hash`, one position
per text line, which is not practical at the scale a single search would
need. Note flippy already applies this idea server-side: `internal/book`
minimaxes every <12-disc position from the 12-disc evaluations.

## 7. Where the leverage actually is

1. **Level choice.** 2.13x per +2 levels dominates everything else. Raising
   the target from 32 to 40 costs ~21x per board; there is no engine change
   in this document that offsets even one level.
2. **Workers = physical cores**, `EDAX_TASKS=1` (§4). Already the case.
3. **`-march=native` build**: ~6% (§1).
4. **Persistent Cassio process** for the interactive ladder: ~2x on that
   path only (§5), plus it removes level-change restarts (§3).
5. **Spend depth at the frontier.** For full-width search,
   `value(parent, d+1) = max over children of -value(child, d)`, and level
   == depth over most of this range. So a ply that is going into the book
   anyway gives its parent a free +1 level via `internal/book`'s minimax.
   Searching a parent one level deeper directly costs ~1.46x; evaluating its
   children (typically 6-10 of them) one level deeper costs ~6x or more — so
   this only pays when the children are wanted for the book in their own
   right, which for a growing book they usually are.

## Reproducing

Scripts used are throwaway; the recipes are short:

- board set: sample lines from `internal/othello/precomputed_boards_12discs.txt`,
  convert `<16 hex black><16 hex white>-<b|w>` (bit i = square i, A1..H8) to
  the OBF line `<64 chars of X/O/-> <X|O>;`.
- timing: `lEdax-x64 -solve <file>.obf -level L -verbose 0 -n-tasks 1`, read
  the trailing `N nodes in M:SS.mmm` line.
- Cassio: `lEdax-x64 -cassio`, write `ENGINE-PROTOCOL ...` lines to stdin,
  read until `ready.`. `empty-hash` sends *no* reply — do not wait for one.
- rebuilds: `docker run --rm -v <edax>:/work -w /work/src gcc:14 ...`
  (`registry-1.docker.io` is on the sandbox allowlist; github.com is not).
