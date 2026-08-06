Task: build a Rust crate, compiled to WASM, that reproduces Edax's evaluation score for a board
bit-exactly at any level <= 10 (levels 11+ need ProbCut, see Tasks 12-18), so the browser can
evaluate positions without a server round-trip. Correctness (bit-exact vs. real Edax) and
performance are the priorities.

This file is the product of a design discussion with the user; the decisions below are settled, not
open questions. Implement as specified, don't re-litigate.

All source-level facts below were read directly from `~/projects/INACTIEF/edax-reversi/src/` (Edax
4.5.1, see [[reference_edax-source-location]]) and from `~/projects/INACTIEF/edax-reversi/data/eval.dat`
(the real weights file), not guessed.

## How to work through this list

The "Task list" section below is a checklist. When picking up this file:

1. Find the first unchecked (`- [ ]`) item, top to bottom. Work on that one only — don't skip ahead or
   batch multiple items into one pass, even if a later item looks easy or related.
2. Do the work for that item only.
3. Check it off (`- [ ]` -> `- [x]`) in the same commit that completes the work — the checkbox flip and
   the code change are not separate commits.
4. One commit per task item. Do not combine two checklist items into a single commit, and do not leave a
   commit half-finishing an item.
5. If a task turns out to need a decision that isn't already settled below, stop and ask rather than
   guessing — see [[user_collaboration-style]].
6. Tasks 12+ and the "possible speed-ups found during code review" section are performance work: a
   plausible-sounding change isn't done until it's measured. Prove the change actually helps using
   `wasm/edax-eval/bench/bench.js` (or, for items that are about correctness/behavior rather than
   raw speed, the relevant test — differential harness for score-affecting changes, unit tests
   otherwise) before checking the box. One commit per item, same as rule 4 above; each commit must
   pass `cargo fmt`/`cargo clippy`/`cargo test` (Task 1's CI wiring) before it's checked off.

## Decisions already made (don't re-litigate)

1. **Scope: level 10 only for v1 — done; levels 0-9 also now in scope.** Tasks 1-10 (the v1
   checklist) are complete; see "v1 (level 10): complete" below. v1's clamp-below-10 behavior
   ("frontend never asks for < 10; the crate should clamp/reject < 10 rather than silently
   default," per [[feedback_coverage-gaps-no-silent-suppression]]) is superseded as of 2026-08-03:
   levels 0-9 are now real, separately-computed levels, not aliases for level 10 — see Task 11.
   Levels 11+ (ProbCut, `eval_sigma`, `selectivity_table`) were deferred out of v1 and remain
   rejected (not silently run as level 10) until Tasks 12-18 (added 2026-08-03) land — see
   decision #2's note on what changes once ProbCut is real.
2. **Equivalence bar: bit-exact**, not "close enough." Every mismatch against real Edax output on the
   differential-test corpus is a bug until proven otherwise (see Task 8). This is realistic here (see
   Background) because level <= 10 in Edax is provably non-selective: no ProbCut, no probabilistic
   forward pruning, so the true minimax value is well-defined and doesn't depend on move ordering, TT
   presence, or search engineering choices — only on correct board logic, correct eval arithmetic, and
   correct depth. This bar stays exactly as-is for level <= 10 forever, levels 0-9 included (Task
   11 is a depth change, not a selectivity change) — Tasks 12-18 (ProbCut etc.) only apply above
   level 10, where Edax itself becomes selective/probabilistic, so bit-exactness there is a
   different (weaker) claim to work out when Task 15 is picked up, not a relaxation of this
   decision.
3. **Language/approach: Rust rewrite**, reading and translating the real Edax C source directly (not a
   clean-room reimplementation from a spec, not an Emscripten build of the C code). Validate against the
   real `lEdax-x64` binary (`EDAX_PATH`), not against `old/` or any prior Go/JS port — per
   [[feedback_verify-against-real-tooling]].
4. **Location: a subdirectory of this repo**, `wasm/edax-eval/`, as its own Cargo crate (own
   `Cargo.toml`, not a workspace member of anything Go). This repo currently has zero Rust/wasm tooling
   and zero frontend build step (`docs/project.md`: "Go `html/template` + vanilla JS/CSS in `static/`,
   no build step") — this crate is the first exception to that, scoped tightly to `wasm/`. This file
   lives at `wasm/edax-eval/TASKS.md`, alongside the crate it governs.
5. **Ship the packed weights, unpack client-side**, exactly like Edax does at load time (`eval_open`,
   `eval.c:565-714`). Do not ship the fully-symmetry-unpacked table (~23.5 MB/ply-set) — ship the packed
   ~11.9 MB raw slice, compressed (see Task 2), and run the same unpacking Edax runs, in Rust, once at
   module init.
6. **Board/move-gen is a third port, not shared code.** `internal/othello` (Go) and `static/board.js`
   (JS) already each reimplement bitboard move generation independently, verified byte-for-byte against
   each other per `docs/project.md`. The Rust port is a third independent implementation of the same
   logic; verify it the same way (differential test against Go/JS output for move generation, and
   against real Edax for full evaluation).
7. **Licensing: `wasm/edax-eval/` is GPLv3.** `wasm/edax-eval/LICENSE` holds the standard GPLv3 text
   (identical to `~/projects/INACTIEF/edax-reversi/LICENSE`), matching decision #3 (this crate is a
   direct translation of GPLv3-licensed Edax source, so the port is a derivative work and inherits the
   license). Scoped to that subtree only — it does not make the rest of the flippy repo GPLv3. Each
   source file added under `wasm/edax-eval/` should carry the standard GPLv3 file header (see the
   "How to Apply These Terms" section at the end of the LICENSE text) crediting Edax as the origin of the
   ported logic.

## Background (facts from source, not assumptions)

### Level 10 is fully non-selective — this is what makes bit-exact tractable

`search.c:161-346` (`search_global_init`) builds `LEVEL[level][n_empties]` = `{depth, selectivity}`.
For `level <= 10` (`search.c:173-179`):

```c
} else if (level <= 10) {
    // sel = 5;   (NO_SELECTIVITY, i.e. full-width, no forward pruning at all)
    if (n_empties <= 2 * level) {
        // dep = n_empties;   (exact solve to game end)
    } else {
        dep = level;          (fixed depth-10 search)
    }
}
```

So level 10 has exactly two regimes, both full-width alpha-beta, no ProbCut:
- **`n_empties <= 20`**: `depth = n_empties` — an **exact solve to the end of the game**. No eval
  function is involved at all; the leaf value is just final disc count (win/loss/draw margin).
- **`n_empties > 20`**: `depth = 10` — a **fixed-depth-10 midgame search**, whose leaves are scored by
  the evaluation function.

This confirms decision #1: `selectivity_table`, `eval_sigma`, and recursive ProbCut (`midgame.c`'s
`USE_PROBCUT` path) are dead code at level <= 10 — this is why v1 could ignore them entirely. They
stop being dead code once Tasks 12-18 add level 11+ support; see "Level 11+ background" below for
where they live in the real source. (Levels 0-9, Task 11, stay inside this same dead-code regime —
only the depth changes, per `search_global_init`'s `level <= 10` branch below.)

### Level 11+ background (from source, for Tasks 12-18)

`search_global_init` (`search.c:161-346`) continues past the level <= 10 case this crate currently
ports: e.g. `level <= 12` sets `sel = 5` (`NO_SELECTIVITY`) for `n_empties <= 21`, `sel = 3` for
`n_empties <= 24`, else `sel = 0` — the selectivity ladder gets more aggressive at higher levels and
lower `n_empties`. `NO_SELECTIVITY = 5` (`search.c:102`); `selectivity_table` (`search.c:104-111`) is
6 entries of `{t, selectivity, percent}`, e.g. `{1.1, 0, 73} ... {999, 5, 100}`. `eval_sigma`
(`eval.c:948-957`) is `sigma = EVAL_A*n_empty + EVAL_B*depth + EVAL_C*probcut_depth`, then
`sigma = EVAL_a*sigma^2 + EVAL_b*sigma + EVAL_c` — the `EVAL_A/B/C/a/b/c` constants live nearby in
`eval.c` and must be read from source, not guessed. `search_probcut` (`midgame.c:288-350`) is the
consumer: a shallow-depth null-window probe gated on `depth >= options.probcut_d && search->selectivity
< NO_SELECTIVITY`, using `eval_sigma` and `selectivity_table[selectivity].t` to size a probcut
window and skip the full-depth search when confident. Endgame stability cutoffs are
`NWS_STABILITY_THRESHOLD[n_empties]` (`search.c:114-121`) gating a `get_stability_fulls`-based check
(`endgame.c:538-540`). Parity-based move-sort weighting is `move.c:302-341` (`w_low_parity`/
`w_mid_parity`/`w_high_parity`, applied via `search->eval.parity & QUADRANT_ID[move->x]`).

### The eval weight table is indexed by *ply* (moves played), not by n_empties directly

`midgame.c:112`: `accumlate_eval(60 - search->eval.n_empties, &search->eval)`. Since
`n_empties + discs_on_board = 64` and the game starts with 4 discs, `60 - n_empties` = number of moves
played so far (ply count from game start, 0-indexed). `Eval_weight` is only populated for
`ply` in `[2, 53]` (`eval.h:60`: `EVAL_WEIGHT[EVAL_N_PLY - 2]` covering "for 2..53"; `eval.c:663`
explicitly skips ply 0 and 1 on load). This range is never actually violated at level 10: the
depth-10 midgame branch only fires when `n_empties > 20` (ply < 40), and depth-10 lookahead lands the
leaf at `ply + 10 <= ... `, staying inside `[2, 53]` for any legal root position. No bounds-clamping
logic needs to be invented — just confirmed with an assertion/test.

`search_eval_0` (`midgame.c:105-121`) is the actual leaf scoring function:

```c
score = accumlate_eval(60 - search->eval.n_empties, &search->eval);
if (score > 0) score += 64; else score -= 64;
score /= 128;
if (score < SCORE_MIN + 1) score = SCORE_MIN + 1;
if (score > SCORE_MAX - 1) score = SCORE_MAX - 1;
```

`accumlate_eval` (top of `midgame.c`, just above `search_eval_0`) sums packed feature weights
(`w->C9[...]`, `w->C10[...]`, `w->S100/S101[...]`, `w->S8x4[...]`, `w->S7654[...]`, `w->S0`) selected by
the board's feature indices — this is the arithmetic that must be ported bit-for-bit, including the
integer rounding (`+= 64; /= 128`) and the `SCORE_MIN+1`/`SCORE_MAX-1` clamp, since C integer division
truncation must match Rust's (both truncate toward zero for `/` on signed ints, so this is safe, but
must not be "fixed" to round-to-nearest by mistake).

### Weights file: what's actually loaded, what we need to ship

`eval.dat` (`~/projects/INACTIEF/edax-reversi/data/eval.dat`, 13,952,436 bytes) layout, from
`eval_open` (`eval.c:565-714`):
- 28-byte header: 2×`int` magic, `int version`, `int release`, `int build`, `double date`.
- Then blocks of `n_w = 114364` `short` (packed, symmetry-reduced weights) per ply. The file physically
  holds 61 such blocks (legacy 60-ply format), but the loader only reads `EVAL_N_PLY = 54` of them
  (`ply = 0..53`) and **skips ply 0 and 1** (`eval.c:663: if (ply < 2) continue;`). So only **52 blocks,
  plies 2..53, are ever used** — a 52 × 114364 × 2 = 11,893,856-byte raw slice out of the file. We do
  not need to ship the trailing ~2 MB of the file at all.
- Per block, the loader unpacks each of the 52 plies into the runtime `Eval_weight` struct
  (`eval.h:47-55`: `C9[19683]`, `C10[59049]`, `S100[59049]`, `S101[59049]`, `S8x4[6561*4]`,
  `S7654[3240]`, `S0`) using `EVAL_PACKED_OFS = {0, 10206, 40095, 69741, 99387, 102708, 106029, 109350,
  112671, 113805, 114183, 114318, 114363}` as offsets into the packed `n_w`-length block, and a set of
  symmetry-unpacking tables built once at startup (`set_eval_packing`, `set_opponent_feature`,
  `eval.c:496-630`) that exploit board mirror symmetry to store ~half the distinct weight values. This
  unpacking must be ported faithfully — it's pure index arithmetic, no floating point, so it's exactly
  reproducible.

**Compression, measured against the real file** (packed plies 2..53 only, before symmetry unpacking):
| encoding | size |
|---|---|
| raw (52 plies × 114364 × 2 bytes) | 11,893,856 B |
| `zstd -19` of raw slice | ~8.46 MB (whole-file baseline, for comparison) |
| delta across ply axis (each weight index vs. previous ply), `zstd -19` | 5,328,957 B |
| delta + byte-plane split (low/high bytes separated), `zstd -19` | 4,812,688 B |
| **transpose (each weight's 52-ply series made contiguous) + delta + byte-plane split, `zstd -19`** | **3,971,643 B** |
| same transform, `gzip -9` (in case zstd decode isn't viable in-browser, see Task 9) | 4,763,088 B |

Recommendation: ship the transpose+delta+byte-plane blob. It roughly halves the naive-compressed size
because adjacent plies' weights are highly correlated (the transform exposes that to the compressor)
and because deltas cluster near zero (byte-plane splitting groups the resulting mostly-zero high bytes
together). This is an offline, one-time transform — see Task 2.

### Licensing

Edax is GPLv3 (`~/projects/INACTIEF/edax-reversi/LICENSE`). This crate is a direct line-by-line
translation of Edax's board/eval/search logic (per decision #3, reading the real source, not a
clean-room spec implementation), making it a derivative work under GPLv3. Serving the compiled `.wasm`
to browsers is "distribution" in copyright terms, which is exactly what GPLv3's copyleft obligations are
triggered by. Resolved per decision #7: `wasm/edax-eval/LICENSE` carries the GPLv3 text, scoped to that
subtree. The rest of the flippy repo (which still has no repo-root LICENSE file) is unaffected.

Note the eval weights themselves (the trained numbers in `eval.dat`, not the code) are Edax's trained
model and are redistributed as-is once shipped as a browser asset — same GPLv3 coverage applies, they're
part of the Program.

### Existing CI test-skip pattern to mirror

`internal/edax/process_test.go` gates real-binary tests on `EDAX_PATH` being set and calls `t.Skip(...)`
when it isn't (`process_test.go:30-36`), rather than excluding the test file from CI by other means. CI
has no `EDAX_PATH` (per [[project_edax-ci-strategy]]), so these tests always skip there and always run
locally. The differential-test harness in Task 8 should use the same pattern — check for `EDAX_PATH` at
the top of the test and skip (not fail, not `#[ignore]`) when unset — so it needs no special CI
exclusion logic beyond what naturally falls out of CI lacking the binary.

## Task list

### v1 (level 10): complete

Tasks 1-10 (crate skeleton + CI wiring, weights extraction tool, board port, weight loader, eval
feature computation + `search_eval_0`, full-width negamax, pass/game-end handling, differential
correctness harness, wasm build, minimal JS wrapper) were the v1 checklist and are all done —
re-confirmed against the actual code in `wasm/edax-eval/` on 2026-08-03 (crate builds, CI/pre-commit/
`test.sh` all wire up `cargo fmt`/`clippy`/`test` + a `wasm32-unknown-unknown` build, `src/board.rs`
has the bitboard port with cross-checked tests, `src/weights.rs`/`src/weights_transform.rs` have the
loader/unpacker, `src/eval.rs` has feature tracking + `search_eval_0`, `src/search.rs` has negamax with
pass handling, `tests/differential.rs` has the `EDAX_PATH`-gated harness, `src/wasm_api.rs` + `js/`
have the JS wrapper). Full detail on what was built and the corrections made along the way lived in
this section; removed 2026-08-03 now that all ten items are checked off and stable — see git history
for the original per-task notes if needed.

### Levels 0-9: extend v1's exact/non-selective solver to the rest of that regime

- [x] 11. **Levels 0-9: real per-level depth, stop clamping to 10.** `search_global_init`'s
      `level <= 10` branch (`search.c:170-179`) already covers levels 0-9 too — same non-selective
      regime as level 10 (`sel = 5`/`NO_SELECTIVITY` unconditionally), just a different depth:
      `dep = n_empties` when `n_empties <= 2 * level` (an exact solve, same shape as level 10's
      `<= 20` cutoff, just scaled to `2 * level`), else `dep = level`; plus a `level <= 0` special
      case where `dep = 0` unconditionally (an immediate `search_eval_0` at the root, no lookahead
      at all). Currently `src/search.rs`'s `solve` ignores the input `level` below 10 entirely — its
      `EXACT_SOLVE_EMPTIES`/`MIDGAME_DEPTH` constants are hardcoded to level 10's numbers, so levels
      1-9 silently run as level 10 (the deliberate v1 clamp, decision #1's old text, now superseded).
      Parameterize both on the real `level` input instead (10 unchanged: `2 * 10 = 20` already
      matches `EXACT_SOLVE_EMPTIES`). Still fully non-selective and bit-exact (decision #2's bar
      applies unchanged) — verify via the existing differential harness (Task 8) run at a few levels
      below 10, not just 10. Update `src/wasm_api.rs`'s doc comment (currently says "Levels 1..=10
      all clamp to identical (real level 10) behavior") and replace/rewrite
      `src/search.rs`'s `levels_below_ten_clamp_to_the_same_result_as_ten` test, which asserts
      exactly the clamp behavior this task removes.

### v2: levels 11+ and further search performance

Split out of "Explicitly deferred" on 2026-08-03, per user request. Opening book usage stays out of
scope for all of these — see the note in "Explicitly deferred" above.

- [x] 12. **Levels 11+ depth/selectivity table.** Extend the depth logic from Task 11 further —
      `src/search.rs`'s depth computation, ported so far only from the `level <= 10` branch of
      `search_global_init` — to cover level 11+, whose selectivity varies by both level and
      `n_empties` (`search.c:161-346`; e.g. `level <= 12`: `sel = 5` for `n_empties <= 21`,
      `sel = 3` for `<= 24`, else `sel = 0` — see "Level 11+ background" above for more). Relax
      `solve`'s `level > 10` rejection (`UnsupportedLevel`) accordingly. This task is just the table
      + threading a `selectivity` value through the search signature — it doesn't need to change any
      scores yet, since nothing consumes selectivity until Task 15.
- [x] 13. **`eval_sigma`.** Port `eval_sigma` (`eval.c:948-957`): `sigma = EVAL_A*n_empty +
      EVAL_B*depth + EVAL_C*probcut_depth`, then `sigma = EVAL_a*sigma^2 + EVAL_b*sigma + EVAL_c`. Read
      the real `EVAL_A/B/C/a/b/c` constants from `eval.c` — don't guess them.
- [x] 14. **`selectivity_table`.** Port the 6-entry `{t, selectivity, percent}` table (`search.c:104-111`,
      `NO_SELECTIVITY = 5` at `search.c:102`) that Task 12's per-level selectivity value indexes into,
      and that Task 15's ProbCut reads `t` from.
- [x] 15. **ProbCut.** Port `search_probcut` (`midgame.c:288-350`): a shallow-depth null-window probe
      that estimates whether a full-depth search would fail high/low, using `eval_sigma` (Task 13) and
      `selectivity_table[selectivity].t` (Task 14) to size the probe window, skipping the full-depth
      search when confident. Gated on `alpha+1==beta` (NWS context), `depth < n_empties` (midgame),
      `selectivity < NO_SELECTIVITY`, and `probcut_level < 2` (recursive ProbCut limit). `probcut_depth =
      2*(depth/4) + (depth&1)` (integer equivalent of `2*floor(probcut_d*depth)+(depth&1)`, `probcut_d =
      0.25`). Only fire when `probcut_depth >= 2`. `RCD = 0.5` (midgame.c:24-26). Add `probcut_level: u32`
      parameter to `negamax` (start 0, probe calls pass `probcut_level+1`); rename `_selectivity` →
      `selectivity`. Decisions made 2026-08-03:
        - **Recursive ProbCut**: yes, matching `USE_RECURSIVE_PROBCUT=true` + `LIMIT_RECURSIVE_PROBCUT(probcut_level<2)`.
          Probe calls pass `selectivity` unchanged; nesting capped at 2 via `probcut_level` counter.
        - **Differential harness for level 11+**: sanity-check only. Keep harness testing levels 5, 8, 10
          (bit-exact, non-selective). Add unit tests that level 11+ returns a score in [-64,64] without
          panicking. No Edax differential comparison for level 11+ until Tasks 16-18 close the search-quality
          gap (TT, parity sorting). Revisit after Task 18.
- [x] 16. **Transposition table.** Add a TT (probe/store keyed on board + depth + alpha/beta bounds) to
      the search. Verify with a regression check that TT presence never changes the score at level <= 10
      (still fully non-selective — decision #2's bit-exact bar still applies there) and that it speeds
      up level 11+ (Task 15) by reusing ProbCut/full-depth results. A TT is an in-memory, per-call search
      cache, not book storage — nothing here should read from or write to the `boards` table or any book
      format (see "Explicitly deferred").
      **Decision:** Direct-mapped 2^14 = 16384 entries (~384 KB), per-call (fresh TT each `solve`
      invocation). Hash: `player * PRIME1 + opponent * PRIME2` (multiplicative mix, 64-bit). Replacement
      policy: depth-preferred (replace when new depth >= stored depth). Cutoff logic (`try_cutoff`):
      fail-high when `lower >= beta`, fail-low when `upper <= alpha`, exact when `lower == upper` in window.
      Store logic: original alpha (not updated alpha) used to classify fail-high/exact/fail-low and store
      correct bounds `(lower, upper)`.
      **Benchmark (level-12 midgame, n_empties=26-28, dep=12 sel=0):** no-TT 618ms vs with-TT 573ms (~7%
      faster). Level-10 bit-exact regression: all 40 corpus positions unchanged vs. no-TT baseline and
      real Edax (differential test). Bench also added `MIDGAME12_CASES` to `bench/bench.js` (discCount
      36 and 38 only — n_empties 28 and 26; excludes n_empties ≤ 24 which hit exact-solve at depth 22-24,
      too slow for benchmark without further endgame search engineering).
- [x] 17. **Endgame-specific stability cutoffs.** Port stability-based pruning for the exact-solve regime
      (`NWS_STABILITY_THRESHOLD[n_empties]` gating a `get_stability_fulls`-based check,
      `search.c:114-121` / `endgame.c:538-540`) — a correctness-preserving pruning technique (doesn't
      change the final minimax value); verify via the differential harness that scores are unchanged
      before/after.
      **Decision:** New `src/stability.rs` module. `EDGE_STABILITY` table (65536 × u8, `OnceLock`) built
      lazily via recursive `find_edge_stable` (port of `board.c:681-737`), using `horizontal_mirror`
      symmetry to halve computation. `get_stable_edge` (pack A1A8/H1H8 columns, look up table, unpack
      A2A7/H2H7) feeds into `get_full_lines` (H/V/D9/D7 full-line bitboards) and `get_spreaded_stability`
      (iterative neighbor propagation). Cutoff in `negamax`: guarded on `!in_midgame && alpha >=
      NWS_STABILITY_THRESHOLD[n_empties]`, computes `SCORE_MAX - 2*get_stability(opponent, player)`, and
      returns immediately on fail-low. Verified: all 45 tests pass including differential harness
      (`matches_real_edax_at_level_10`, `matches_real_edax_at_levels_below_ten`) — scores unchanged.
- [ ] 18. **Parity-based move sorting -- implemented, benchmarked, regressed; not adopted.** Ported the
      parity heuristic from `move.c:302-341` (`QUADRANT_ID`/`initial_parity` mirroring `search_setup`,
      `search.c:501-513`; `parity_weight` bucket table mirroring `w_low_parity`/`w_mid_parity`/
      `w_high_parity`, `move.c:273-284`) as a small tiebreak layered on top of the existing
      ascending-opponent-mobility sort in `src/search.rs`'s `negamax` (`sort_key = opp_mobility * 16 -
      parity_bonus`, scaled so parity only ever breaks exact-mobility ties, matching how Edax's own
      weights make `w_mobility` dominate `w_low_parity`). Bit-exactness held (differential harness,
      `EDAX_PATH` set locally, still passes at levels 5/8/10) -- but `bench/bench.js`'s full corpus
      measured **~2.85s vs. a ~2.10s baseline (~35% slower), reproduced across two independent build/
      measure cycles** (both threading parity through `negamax` as an extra recursive parameter, and a
      second version recomputing it fresh from the board per node instead -- both equally slow, so the
      regression isn't extra-parameter call overhead, it's that computing `initial_parity` (4 masked
      popcounts) on every internal node costs more than the tiebreak saves on this engine, which already
      has a TT + stability cutoffs + NWS/PVS + mobility ordering doing most of the pruning work). Per
      "how to work through this list" rule 6, a change that doesn't measurably help doesn't get checked
      off -- reverted (`git checkout -- src/search.rs`) rather than left as dead-weight regressed code.
      Left unchecked, annotated, so this exact experiment isn't blindly redone; a future attempt would
      need a cheaper parity computation (e.g. only computing it when the mobility sort actually has a
      tie, or maintaining it incrementally without widening `negamax`'s signature) to be worth retrying.

### Possible speed-ups found during code review (flagged 2026-08-03)

Spotted by inspection while reading `src/`, not measured — each needs investigating and, where a real
change is proposed, benchmarking via `wasm/edax-eval/bench/bench.js` before deciding whether to act
(per "how to work through this list" rule 6). A couple of these were already investigated enough while
writing this list to know the outcome; noted inline so they aren't re-investigated from scratch.

- [x] 19. **`Board::get_moves`/`get_some_moves` inlining.** A/B benchmarked (`bench/bench.js`'s full
      corpus, `wasm32-unknown-unknown --release` build, `lto = true, codegen-units = 1` unchanged):
      adding `#[inline]` to both measured consistently faster across two independent build/measure
      rounds (6 runs each) -- ~2059ms avg with the attribute vs. ~2129ms avg without (~3.3% faster);
      five of six candidate runs beat every baseline run, and the one outlier still came in under the
      baseline's own max. So LTO + `codegen-units = 1` does *not* fully substitute for the explicit hint
      here (the task's caveat that it "often" does was a real possibility, not the outcome) -- kept both
      `#[inline]` attributes (`src/board.rs`).
- [ ] 20. **`Board::get_flip` alternate implementations -- attempted and benchmarked 2026-08-04;
      no measurable win, reverted.** Current `get_flip` (`src/board.rs:99-216`) is
      a hand-written 8-direction "dumb-fill" (fixed 5-iteration unrolled loop + edge-mask per direction,
      with an early exit if the first step is empty). Edax ships several alternate flip algorithms as
      swappable `flip_*.c` files (`board.c:31-63` dispatches on `MOVE_GENERATOR`): `flip_kindergarten.c`
      (Edax's actual default/portable fallback when no `MOVE_GENERATOR` is set — table-lookup based,
      likely the fairest baseline to compare against), `flip_carry_64.c` (`OUTFLANK`/`FLIPPED` byte-array
      lookups + carry propagation, `MOVE_GENERATOR_CARRY`), `flip_bitscan.c`, `flip_roxane.c`. The
      SSE/AVX/NEON variants (`flip_sse*.c`, `flip_avx*.c`, `flip_neon*.c`) aren't directly relevant
      (x86/ARM SIMD intrinsics, not portable to `wasm32-unknown-unknown`'s SIMD128 as-is, though the
      general *approach* of some may still translate). Port and bench 1-2 of the most promising against
      the current implementation; keep whichever wins, including possibly the current one.

      **Investigation (2026-08-03):** confirmed each of `flip_kindergarten.c`/`flip_carry_64.c`/
      `flip_bitscan.c`/`flip_roxane.c` is not a small, generic algorithm -- each is 65 hand-specialized
      per-square functions (`grep -c '^static unsigned long long flip_'` = 65 in every file, i.e. one
      function per square plus `flip_pass`), each baking in per-square/per-direction magic bit-masks and
      multiply constants (e.g. `flip_A1`'s vertical term: `(O & 0x0001010101010100) * 0x0002040810204000
      >> 57`), backed by large precomputed byte tables (`flip_kindergarten.c`'s `OUTFLANK[8][64]`/
      `FLIPPED[8][144]`). Transcribing that by hand for this crate (~2000-2600 lines per file) is exactly
      the kind of bit-exact-critical, easy-to-get-subtly-wrong work this project has been careful about
      elsewhere (decision #3: read and translate real source, not reimplement from a spec) -- but at this
      volume of hand-typed hex constants, transcription risk is itself a correctness risk, not just a time
      cost. A *generic* (non-per-square) reimplementation of the same underlying technique was also
      considered: it needs, per line direction, a flip-lookup keyed on (position, player-byte,
      opponent-byte) -- a naive combined table is 8 * 256 * 256 = 524,288 entries, too large to be
      obviously cache-friendly in wasm linear memory (plausibly *slower* than the current 5-iteration
      dumb-fill, not faster), and Edax's own two-stage `OUTFLANK`/`FLIPPED` design that avoids that size
      requires precisely re-deriving a bit-packing scheme this investigation didn't have confidence in
      getting right without further dedicated study. Given Task 18 already showed a "should obviously
      help" heuristic regress performance ~35% once actually measured, committing this much
      transcription/design risk on an unmeasured hunch isn't a good trade. Left unattempted and unchecked
      (current dumb-fill implementation stays) rather than rushed; a future pass with more time budget
      could prototype the generic two-stage table design as a separate, smaller experiment before
      committing to a full port.

      **Attempt (2026-08-04):** the 2026-08-03 investigation's conclusion (per-square transcription is too
      risky, the naive generic table is too big) still held -- this sandbox pass additionally had no access
      to `~/projects/INACTIEF/edax-reversi/src/` at all, so transcribing Edax's own `flip_*.c` was not an
      option regardless. Instead, ported the *general technique* those files chase (fewer sequential steps
      than a straight dumb-fill) from first principles rather than from Edax source: the "Kogge-Stone"
      doubling occluded-fill algorithm, a well-known generic bitboard sliding-piece technique from chess
      programming (not Edax-specific, so no per-square hex constants to transcribe and no GPL-derivation
      question). Per direction: `gen` starts at the origin square and floods through the opponent mask via
      doubling shifts (1, 2, 4 squares, i.e. 3 steps covering a run of up to 7 squares) instead of the
      current 6 sequential 1-square steps, with the propagator mask re-ANDed with its own shifted copy each
      step (`pro &= pro << shift`) so a jump is only taken through cells already confirmed reachable --
      exactly the standard algorithm, adapted from "fill through empty squares" to "fill through opponent
      squares, then check one more cell for a player bracket." Verified bit-exact: all 45 existing tests
      passed unmodified, including both differential-harness tests against the real `lEdax-x64` binary
      (`EDAX_PATH` was set in this sandbox) -- `matches_real_edax_at_level_10` and
      `matches_real_edax_at_levels_below_ten`.
      **Benchmark** (`bench/bench.js` full corpus, `wasm32-unknown-unknown --release`, same
      `lto = true, codegen-units = 1` profile): three independent alternating build/measure rounds (3, 3,
      then 6 runs per side, 12 runs each total) -- baseline avg 2057.9ms vs. candidate avg 2070.1ms (candidate
      ~0.6% *slower*), with individual runs swinging ~2000-2290ms (~5-7% run-to-run noise) on both sides.
      No round showed a consistent winner either way. Per "how to work through this list" rule 6, this is not
      a measured win, so it doesn't get checked off -- reverted (`git checkout -- src/board.rs`) rather than
      landed as a no-benefit rewrite of bit-exact-critical code. Plausible reason the doubling algorithm
      didn't pay off here: it trades fewer sequential steps for more live values per direction (`gen` and
      `pro` both carried across 3 steps, vs. a single accumulator in the dumb-fill), and the *existing*
      early-exit (`if gen != 0`, skipping the whole direction when the immediate neighbor isn't opponent) already
      prunes the common case cheaply -- on an 8-wide board the total step count saved (6 to 3) is small
      enough that the extra register/instruction overhead of maintaining `pro` erased the gain. Left
      unchecked, current dumb-fill implementation unchanged; a future attempt would need either a genuinely
      different algorithmic win (not just fewer same-cost steps) or profiling tooling this sandbox doesn't
      have to explain *why* it's a wash before it's worth another try.

      **Correction (2026-08-04, same day):** the 2026-08-04 attempt above claimed
      `~/projects/INACTIEF/edax-reversi/src/` was unavailable in this sandbox -- that was wrong. It was
      checked as the sandbox *agent* user's home (`/home/agent/projects/...`), which doesn't have it; the
      real source is at `/home/luuk/projects/INACTIEF/edax-reversi/src/` (this project's owner's home,
      matching what every other reference to `~/projects/INACTIEF/edax-reversi/src/` elsewhere in this file
      means) and was accessible the whole time. Redone properly below with the actual `flip_*.c` files.

      **Second attempt, with real source (2026-08-04):** read `flip_kindergarten.c`, `flip_carry_64.c`,
      `flip_bitscan.c`, `flip_roxane.c`, `flip_bmi2.c`, and (load-bearing) `generate_flip.c` -- the tool
      that *writes* `flip_kindergarten.c`. `generate_flip.c` exists precisely because the 65 per-square
      functions the earlier investigation found are a mechanical unrolling of one generic algorithm
      (`h_to_line`/`v_to_line`/`d7_to_line`/`d9_to_line`/`outflank`/`flip`, plus per-square mask/index
      derivation in `main`) -- so the "65 hand-specialized functions, too risky to transcribe" framing was
      right about the *shipped* file but wrong that a generic port wasn't available: the generator's own
      formulas, parameterized by the actual square at runtime instead of unrolled at C-compile-time, port
      cleanly and don't require touching the per-square code at all. Ported two candidates plus re-added the
      first attempt's Kogge-Stone for a fair three-way comparison, all in a new `src/flip_variants.rs`:
        - **`flip_kindergarten`**: the generator's algorithm as above. `OUTFLANK[8][64]`/`FLIPPED[8][144]`
          copied byte-for-byte via a parsing script (not hand-typed), cross-checked identical against the
          same tables in the independently-written `flip_bmi2.c`. Gather/scatter derived from the generator
          source, not guessed.
        - **`flip_carry`**: the "propagate a carry through contiguous opponent bits via `+1` to find the
          first non-opponent cell, then check if it's a player disc" technique shared by `flip_carry_64.c`
          and `flip_bitscan.c` (the latter's `outflank_right` macro handles the reverse direction via a
          leading-zero-count, which `wasm32` has as a fast native instruction with no CPU-feature-check
          needed, unlike x86 `lzcnt` -- so this ports that form for both directions instead of
          `flip_carry_64.c`'s CONTIG-table alternative for the reverse direction).
        - **`flip_roxane`: examined, not ported.** Same tableless-arithmetic family as `flip_carry`, but
          `flip_roxane.c`'s own header notes its square numbering is "inverted compared to Edax's", and a
          worked example (A1's SE-direction mask, expected to cover the full 8-cell corner-to-corner
          diagonal) only resolved to a 6-cell mask under a from-scratch re-derivation in Edax's own
          numbering -- a real, demonstrated ambiguity, not a hand-wave. Since `flip_carry` already tests
          this algorithm family's hypothesis, re-deriving Roxane's specific inverted-numbering variant for
          no new algorithmic coverage wasn't worth the transcription/off-by-one risk.
        - SSE/AVX/NEON/SVE/BMI2/AVX512 variants: still out of scope, confirmed again by actually reading
          `flip_bmi2.c` this time -- it uses `_bextr_u32`/`_pext_u64`/`_pdep_u64`, x86 BMI2 hardware
          instructions with no `wasm32-unknown-unknown` equivalent (a software PEXT/PDEP emulation was
          considered and rejected: it would just reduce to reimplementing the same
          gather/scatter-by-multiply-or-loop approach `flip_kindergarten`/`flip_carry` already cover).

      **A real bug, caught by the differential harness:** first version of `flip_kindergarten` failed
      `matches_independent_reference_across_random_self_play_games`-style self-play testing (a new
      `check_against_reference` test comparing every legal move's flip result against `Board::get_flip`
      across 200 random self-play games) on the very first run, then again after fixing the first bug --
      concretely confirming decision #2's "every mismatch is a bug" bar caught real errors here, not just in
      principle: (1) the multiply-based gather trick used for vertical/diagonal lines only produces the
      assumed bit ordering when the line's start bit is byte-aligned (true for vertical, `step=8`); for
      diagonals (`step=9`/`7`) the same trick produces a *rotated or fully bit-reversed* ordering depending
      on the diagonal's own start-bit alignment mod 8 (confirmed by hand for two different diagonals -- one
      gave a simple "+1 offset", another a full reversal) -- exactly why Edax's own generated code uses a
      different precomputed unpack array per diagonal group rather than one formula; fixed by gathering
      diagonals with an explicit, unambiguous per-position loop instead of the multiply trick. (2) A shared
      `MID_MASK` (excluding both edge rows *and* edge columns, correct for diagonals since their two
      endpoints can sit on any of the four edges) was reused for the vertical line too, where it's wrong: a
      column has a *constant* column, so excluding "column A or H" zeroes the entire gather whenever the
      played square is itself in column A or H, not just the column's own endpoints. Split into a
      diagonal-only `MID_MASK` and a vertical-only `ROW_MID_MASK` (excluding only the edge rows). After both
      fixes: 200/200 self-play games clean, and the full test suite (47 tests, including both
      `matches_real_edax_at_level_10` / `matches_real_edax_at_levels_below_ten` differential tests against
      the real `lEdax-x64` binary, `EDAX_PATH` set in this sandbox) passed with each candidate wired in as
      `Board::get_flip` in turn.

      **Benchmark** (`bench/bench.js` full corpus, `wasm32-unknown-unknown --release`, same
      `lto = true, codegen-units = 1` profile, all four numbers measured in the same session for a fair
      comparison -- 6 runs each, baseline measured twice, at the start and as a bookend, to check for
      machine-load drift):

      | candidate | avg (12 baseline / 6 each candidate runs) | vs. baseline |
      |---|---|---|
      | baseline (current dumb-fill) | 2047.5ms | -- |
      | Kogge-Stone (first attempt, re-measured) | 2089.9ms | +2.1% slower |
      | `flip_carry` (`flip_carry_64.c`/`flip_bitscan.c` technique) | 2178.4ms | +6.4% slower |
      | `flip_kindergarten` (Edax's actual default portable algorithm) | 3127.2ms | +52.7% slower |

      Baseline's two rounds (2061.2ms, 2033.8ms) were close enough to rule out significant drift.
      `flip_kindergarten` is dramatically slower, not a close call: the explicit per-position gather/scatter
      loops needed to sidestep the diagonal bit-ordering bug (4 small loops per call: gather-mid, gather-full,
      and two scatters) cost far more than the table lookups they enable save, on top of two `OnceLock`
      diagonal-mask lookups per call. `flip_carry` and Kogge-Stone are both modestly slower, consistent with
      the first attempt's finding: the current dumb-fill's cheap `if gen != 0` early-exit already prunes the
      common (no-flip-this-direction) case well, so algorithms trading fewer steps for more live
      state-per-direction (carry: a `mask` lookup plus wrapping add/sub per direction; Kogge-Stone: `gen`
      *and* `pro` carried across 3 doubling steps) don't come out ahead on this 8-wide board. None of the
      three beat baseline, so per rule 6 none get checked off -- `Board::get_flip` itself stays the dumb-fill.
      A future attempt would need either real profiling (unavailable in this sandbox) to find an actual
      bottleneck worth targeting, or a genuinely different algorithmic idea, not another "fewer steps, more
      state" trade in this same family.

      **Update (2026-08-04, same day): the two bugs found above are fixed, with dedicated regression tests,
      and `src/flip_variants.rs` is kept in the tree** (initially reverted per Task 18's "don't leave a
      non-winning implementation as dead code" precedent, then explicitly requested back with fixes and
      regression tests). Reframed accordingly: `Board::get_flip`'s own doc comment already treats Edax's
      `flip_slow.c` as a kept-forever verification oracle (ported as `Board::get_flip` itself, not a
      performance path); `flip_kindergarten`/`flip_carry`/`flip_kogge_stone` now serve the same role --
      three independent implementations `Board::get_flip` must keep agreeing with, none of them wired in as
      the active implementation (that verdict above is unchanged). The module is `#[cfg(test)]`-gated so it
      never ships in the release wasm build. Regression tests added, one per bug, each a minimal fixture
      (not just relying on the general 200-random-game self-play fuzzer to catch a re-introduction
      probabilistically):
        - `kindergarten_diagonal_bit_ordering_regression`: playing F2 (square 13) on
          `Board { player: 34494480384, opponent: 68988960768 }` must flip only E3 (the `d7` diagonal
          F2-E3-D4, bracketed by a player disc at D4) -- the buggy multiply-based diagonal gather produced
          no flip at all here.
        - `kindergarten_vertical_edge_column_regression`: playing A6 (square 40, column A) on
          `Board { player: 4553165715441500290, opponent: 9245298939280634884 }` must flip B6, C6
          (horizontal) and A7 (vertical, bracketed by a player disc at A8) -- the buggy shared `MID_MASK`
          zeroed the entire vertical gather for column A, silently dropping the A7 flip.
      Both assert against the real fixture boards used to first catch each bug during development, and both
      cross-check the expected value against `Board::get_flip` itself (the already-differential-verified
      reference) before checking `get_flip_kindergarten` against it, so a future edit to either the fixture
      or the reference can't silently make the test vacuous. Full suite (50 lib tests, up from 47, plus both
      real-Edax differential tests) passes; `cargo fmt`/`cargo clippy --all-targets -- -D warnings` clean
      (aside from the pre-existing, unrelated `stability.rs` clippy warnings noted in the first 2026-08-04
      attempt above, confirmed present on a clean checkout of this same commit's parent).
- [x] 21. **`init_features` doc-string / player-opponent parity swap — confirmed correct, doc comment
      clarified.** Verified against real Edax source (`eval.c:757-762`, `eval_set`'s portable path):
      Edax itself swaps `board->player`/`board->opponent` when `eval->n_empties` is odd before computing
      features (`if (eval->n_empties & 1) { b.player = board->opponent; ... }`), for exactly the reason
      this crate's `init_features`/`search_eval_0` doc comments state — the weight table is trained
      per-ply against a fixed "color 0" convention, not a naively mover-relative one. This is not
      "different weights for black vs. white to move" (there's one shared `EVAL_WEIGHT` table per ply,
      not two) — it's a parity-based relabeling of which physical color counts as feature-color-0 for
      that ply, matching Edax exactly. `init_features`'s doc comment (`src/eval.rs`) now says this
      explicitly (the one-shared-table point wasn't previously spelled out there, only in this TASKS.md
      entry) — no behavior change.
- [x] 22. **`update_features`/`undo_features`'s `mover_is_eval_player` parameter — confirmed matches
      Edax, code comment added.** Edax dispatches on `eval->n_empties & 1` to pick between two
      separately-coded functions, `eval_update_0`/`eval_update_1` (`eval.c:782-876`), whose bodies differ
      in exactly the sign/magnitude constants this crate's `mover_is_eval_player ? (-2,-1) : (-1,1)`
      branch computes (verified line-by-line: `eval_update_0` is placed-square `-2x` / flipped `-1x`
      each, matching `mover_is_eval_player = true`; `eval_update_1` is placed `-1x` / flipped `+1x`,
      matching `mover_is_eval_player = false`). Both approaches branch once per move (Edax picks a
      function; this crate picks two constants), so there's no extra per-square branching in either —
      this crate's version is arguably better factored (one shared loop instead of two hand-duplicated
      ones). `update_features`'s doc comment (`src/eval.rs`) now records this equivalence — no behavior
      change.
- [x] 23. **`negamax`'s `in_midgame`/`mover_is_eval_player` — hoisted as parameters, measured a small
      real win.** Both threaded through as caller-supplied `bool` parameters instead of recomputed at
      every node: `in_midgame` (`depth < n_empties`) is provably invariant across the whole search tree
      from the root (every real move decrements both `depth` and `n_empties` by 1 in lockstep, a pass
      decrements neither, so it never changes after the root) and just passes straight through every
      recursive call unchanged, except ProbCut's probe calls, which search the same board to a different
      depth (`probcut_depth`) and so recompute it fresh at the call site (`probcut_depth < n_empties`) --
      still only once per probe, not once per node in the probe's subtree. `mover_is_eval_player`
      (`n_empties % 2 == 0`) flips on every real move (fixed parity) and carries over unchanged across a
      pass and across ProbCut probes (same board, same `n_empties`). `solve` computes both once at the
      root. Bit-exactness held (differential harness still passes at levels 5/8/10). Benchmarked
      (`bench/bench.js`'s full corpus) across two independent build/measure rounds (3 runs each): ~2030ms
      avg with hoisting vs. ~2060ms avg without (~1.5% faster), the candidate winning 5 of 6 paired
      comparisons -- smaller than Task 19's ~3.3% (matching the task's own "minor win at best"
      expectation) but real and consistent enough across both rounds to keep.
- [x] 24. **NWS/PVS — already implemented, this concern is stale.** `negamax` already does a null-window
      search for non-first candidates with a full-window re-search when the score improves alpha without
      proving a cutoff (`src/search.rs:174-188`, commit `953e6b0`, predates this TASKS.md update). No
      action needed.
- [ ] 25. **`i32` vs. `u64` in hot paths — investigated, not attempted this pass (expected-null result
      doesn't justify the refactor risk).** Several counters/indices in `src/eval.rs`/`src/search.rs`
      (feature values, `n_empties`, `ply`, mobility counts) are `i32` where the underlying quantity is
      small and non-negative; investigate whether `u32`/`u64` changes codegen favorably on `wasm32`
      (native word size, sign-extension avoidance) — bench-driven, not a priori, since small-int
      arithmetic on wasm is often identical regardless of signedness and this may turn out to be a
      non-issue.

      **Investigation (2026-08-03):** two things narrow this down before any code change.
      First, "feature values" (`features: &mut [i32; 46]`) turn out not to be a candidate at all --
      unlike `n_empties`/mobility counts, feature values genuinely go negative during incremental
      updates (`update_features`'s `sq_delta`/`flip_delta` are `-2`/`-1`/`+1`; see Task 22's doc
      comment), so they must stay signed; the task's premise ("small and non-negative") only holds for
      `n_empties`/`ply`/mobility counts, not features. Second, and more load-bearing: `wasm32`'s only
      32-bit integer type at the instruction level is `i32` -- Rust's `i32` and `u32` both compile to
      the *same* wasm `i32` value type (there's no separate `u32` register or "native word size" to
      gain from switching), so "native word size" isn't actually in play here as the task speculated.
      The only place signed vs. unsigned choice changes the actual emitted instruction is division,
      remainder, right-shift, and ordered comparison (`i32.div_s`/`div_u`, `shr_s`/`shr_u`, `lt_s`/
      `lt_u`, ...), and every wasm runtime lowers both variants of each to the same-cost native x86/ARM
      instruction (signed and unsigned compare/divide are equally cheap on real hardware) -- there's no
      general reason to expect a difference, unlike Task 19's inlining result (a genuine LLVM
      cost-model/inliner-heuristic effect, not an ISA-level one). Retyping `n_empties` alone would still
      require also retyping `depth` (they're compared directly, `depth < n_empties`/`depth ==
      n_empties`, which Rust doesn't allow across `i32`/`u32` without casts at every site), which rules
      out a small isolated change -- it ripples into `negamax`, `depth_and_selectivity`, `final_score`,
      `init_features`, `eval_from_features`, `eval_sigma`, and the TT's `depth: i32` field. Given Task
      18 already showed real measurement is necessary (a "should help" heuristic regressed ~35%) but
      also that a wide, mechanical, multi-file signature change is exactly the kind of work where a
      transcription slip is easy to introduce in bit-exact-critical code, and the a priori expected
      result here is null (not just "small win, unproven" like Task 23, but "no plausible mechanism for
      a win" on this target), this wasn't attempted. Left unchecked; would need a wasm-level codegen
      inspection tool (none available in this sandbox) showing a concrete instruction-selection
      difference before it's worth the refactor risk.

## Explicitly deferred (do not build in this pass)

- Any frontend UI/page wiring.
- Opening book usage inside the wasm evaluator — this is a raw position evaluator, not a book lookup;
  how it relates to the existing `boards` table / book (if at all) is unspecified and out of scope.
  This stays out of scope even for Task 16's transposition table below: a TT is an in-memory,
  per-call search cache, not book storage — nothing in Tasks 12-18 should read from or write to the
  `boards` table or any book format.

(Levels 11+/ProbCut/`eval_sigma`/`selectivity_table` and TT/stability-cutoffs/parity-sorting were
listed here until 2026-08-03; they're now in scope as Tasks 12-18 below. Levels 0-9 were never
listed here — they were a clamp behavior per decision #1, not a deferred feature — but are now
also in scope, as Task 11.)
