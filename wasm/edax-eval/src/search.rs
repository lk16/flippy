// wasm/edax-eval: a Rust/WASM port of Edax's board, evaluation, and search
// logic, so browsers can reproduce Edax's evaluation score for a position
// without a server round-trip.
//
// Copyright (C) 2026  Luuk Verweij
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//
// This crate is a derivative work, translating board/evaluation/search logic
// from Edax 4.5.1 (https://github.com/abulmo/edax-reversi), also licensed
// under GPLv3.

//! Negamax alpha-beta search covering `search_global_init`'s level regimes (`search.c:161-346`).
//! Level 0: depth 0; level L (1-10): exact solve when `n_empties <= 2 * L`, else fixed depth L.
//!
//! Not a line-by-line port of Edax's `midgame.c`/`endgame.c` search functions: at full width the
//! minimax value depends only on position, depth, and eval, so a plain negamax suffices; verified
//! by the differential harness against the real `lEdax-x64` binary.
//!
//! Load-bearing C detail: a forced pass does not consume depth (`midgame.c:632` recurses with
//! `depth` unchanged; `depth - 1` only on a real move, `midgame.c:662`) — getting this backwards
//! would search a different depth than Edax and diverge from its exact score.

use crate::board::Board;
use crate::eval::{eval_from_features, eval_sigma, init_features, undo_features, update_features};
use crate::weights::EvalWeight;

const SCORE_MIN: i32 = -64;
const SCORE_MAX: i32 = 64;

/// TT size used by [`solve`]: `2^TT_SIZE_LOG2` entries (~384 KB), fits WASM's default memory.
const TT_SIZE_LOG2: u32 = 14;

/// One transposition-table entry, mirroring `HashData` (`hash.h:22-53`); `lower`/`upper` are
/// proven bounds, valid when `key` matches the board hash.
#[derive(Clone, Copy)]
struct TtEntry {
    key: u64,
    depth: i32,
    selectivity: u32,
    lower: i32,
    upper: i32,
}

/// Direct-mapped transposition table (port of `HashTable` + `search_TC_NWS`, `hash.h:68-77` /
/// `search.c:1240-1256`); created once per [`solve`] call.
struct TranspositionTable {
    entries: Box<[TtEntry]>,
    mask: usize,
}

impl TranspositionTable {
    fn new(size_log2: u32) -> Self {
        let size = 1usize << size_log2;
        let empty = TtEntry {
            key: 0,
            depth: -1,
            selectivity: 0,
            lower: SCORE_MIN,
            upper: SCORE_MAX,
        };
        TranspositionTable {
            entries: vec![empty; size].into_boxed_slice(),
            mask: size - 1,
        }
    }

    fn board_hash(board: &Board) -> u64 {
        // Multiplicative mix of player and opponent bitboards.
        board
            .player
            .wrapping_mul(0xCBF2_9CE4_8422_2325_u64)
            .wrapping_add(board.opponent.wrapping_mul(0x517C_C1B7_2722_0A95_u64))
    }

    /// `search_TC_NWS` (`search.c:1240-1256`): return a proven-bound cutoff without searching.
    /// `lower >= beta` (not `alpha < lower`) is critical outside NWS: `alpha < lower < beta`
    /// would return a lower bound as if it were an exact score.
    fn try_cutoff(
        &self,
        board: &Board,
        depth: i32,
        selectivity: u32,
        alpha: i32,
        beta: i32,
    ) -> Option<i32> {
        let key = Self::board_hash(board);
        let e = self.entries[key as usize & self.mask];
        if e.key == key && e.selectivity >= selectivity && e.depth >= depth {
            if e.lower >= beta {
                return Some(e.lower); // fail-high: true score ≥ lower ≥ beta
            }
            if e.upper <= alpha {
                return Some(e.upper); // fail-low: true score ≤ upper ≤ alpha
            }
            if e.lower == e.upper {
                return Some(e.lower); // exact score; alpha < lower = upper < beta → safe to return
            }
        }
        None
    }

    /// Stores a search result; depth-preferred replacement.
    fn store(
        &mut self,
        board: &Board,
        depth: i32,
        selectivity: u32,
        alpha: i32,
        beta: i32,
        score: i32,
    ) {
        let key = Self::board_hash(board);
        let idx = key as usize & self.mask;
        let e = self.entries[idx];
        if e.key != key || e.depth <= depth {
            // `alpha` must be the *original* alpha (before the search loop raised it), else
            // fail-high results store upper = score instead of SCORE_MAX, breaking future probes.
            let (lower, upper) = if score >= beta {
                (score, SCORE_MAX) // fail-high: proven lower bound
            } else if score > alpha {
                (score, score) // exact: both bounds tight
            } else {
                (SCORE_MIN, score) // fail-low: proven upper bound
            };
            self.entries[idx] = TtEntry {
                key,
                depth,
                selectivity,
                lower,
                upper,
            };
        }
    }
}

/// `NO_SELECTIVITY` from `search.c:102`: full-width search, no forward pruning.
pub(crate) const NO_SELECTIVITY: u32 = 5;

/// Port of `struct Selectivity` (`search.h:26-30`); `t` sizes ProbCut's null-window probe,
/// `percent` is the nominal probability the true score falls inside the window.
#[derive(Clone, Copy)]
#[allow(dead_code)]
pub(crate) struct Selectivity {
    pub t: f64,
    pub level: u32,
    pub percent: u32,
}

/// Port of `selectivity_table` (`search.c:104-111`), indexed by [`depth_and_selectivity`]'s
/// selectivity; index [`NO_SELECTIVITY`] is full-width (no ProbCut).
pub(crate) const SELECTIVITY_TABLE: [Selectivity; 6] = [
    Selectivity {
        t: 1.1,
        level: 0,
        percent: 73,
    },
    Selectivity {
        t: 1.5,
        level: 1,
        percent: 87,
    },
    Selectivity {
        t: 2.0,
        level: 2,
        percent: 95,
    },
    Selectivity {
        t: 2.6,
        level: 3,
        percent: 98,
    },
    Selectivity {
        t: 3.3,
        level: 4,
        percent: 99,
    },
    Selectivity {
        t: 999.0,
        level: 5,
        percent: 100,
    },
];

/// Levels above 60 are outside the range `search_global_init` (`search.c:161-346`) defines.
#[derive(Debug, PartialEq, Eq)]
pub struct UnsupportedLevel(pub u32);

impl std::fmt::Display for UnsupportedLevel {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "unsupported edax level {} (levels 0-60 are supported)",
            self.0
        )
    }
}

impl std::error::Error for UnsupportedLevel {}

/// Direct port of `search_global_init` (`search.c:161-346`): `(depth, selectivity)` for `level`
/// and `n_empties`; exact solve when `depth == n_empties`, midgame otherwise.
fn depth_and_selectivity(level: u32, n_empties: i32) -> (i32, u32) {
    let lv = level as i32;
    let mut dep = n_empties;
    let mut sel = NO_SELECTIVITY;

    if level == 0 {
        dep = 0;
    } else if level <= 10 {
        if n_empties > 2 * lv {
            dep = lv;
        }
    } else if level <= 12 {
        if n_empties <= 21 {
        } else if n_empties <= 24 {
            sel = 3;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level <= 18 {
        if n_empties <= 21 {
        } else if n_empties <= 24 {
            sel = 3;
        } else if n_empties <= 27 {
            sel = 1;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level <= 21 {
        if n_empties <= 24 {
        } else if n_empties <= 27 {
            sel = 3;
        } else if n_empties <= 30 {
            sel = 1;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level <= 24 {
        if n_empties <= 24 {
        } else if n_empties <= 27 {
            sel = 4;
        } else if n_empties <= 30 {
            sel = 2;
        } else if n_empties <= 33 {
            sel = 0;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level <= 27 {
        if n_empties <= 27 {
        } else if n_empties <= 30 {
            sel = 3;
        } else if n_empties <= 33 {
            sel = 1;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level < 30 {
        if n_empties <= 27 {
        } else if n_empties <= 30 {
            sel = 4;
        } else if n_empties <= 33 {
            sel = 2;
        } else if n_empties <= 36 {
            sel = 0;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level <= 31 {
        if n_empties <= 30 {
        } else if n_empties <= 33 {
            sel = 3;
        } else if n_empties <= 36 {
            sel = 1;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level <= 33 {
        if n_empties <= 30 {
        } else if n_empties <= 33 {
            sel = 4;
        } else if n_empties <= 36 {
            sel = 2;
        } else if n_empties <= 39 {
            sel = 0;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level <= 35 {
        if n_empties <= 30 {
        } else if n_empties <= 33 {
            sel = 4;
        } else if n_empties <= 36 {
            sel = 3;
        } else if n_empties <= 39 {
            sel = 1;
        } else {
            dep = lv;
            sel = 0;
        }
    } else if level < 60 {
        if n_empties <= lv - 6 {
        } else if n_empties <= lv - 3 {
            sel = 4;
        } else if n_empties <= lv {
            sel = 3;
        } else if n_empties <= lv + 3 {
            sel = 2;
        } else if n_empties <= lv + 6 {
            sel = 1;
        } else if n_empties <= lv + 9 {
            sel = 0;
        } else {
            dep = lv;
            sel = 0;
        }
    }
    // level >= 60: dep = n_empties, sel = 5 (initial values, unchanged)

    (dep, sel)
}

/// Final score of a board with no legal moves for either side, from `player`'s view; empties go
/// to the leader (a tie splits them). Port of `board_solve` (`endgame.c:55-65`); also correct for
/// `n_empties == 0` (`search_solve_0`, `endgame.c:90-95`, is the same formula).
fn final_score(player_discs: i32, n_empties: i32) -> i32 {
    let score = player_discs * 2 - SCORE_MAX;
    let diff = score + n_empties;
    match diff.cmp(&0) {
        std::cmp::Ordering::Equal => 0,
        std::cmp::Ordering::Greater => diff + n_empties,
        std::cmp::Ordering::Less => score,
    }
}

/// Port of `NWS_midgame` (`midgame.c:590-699`): negamax to `depth` *real* moves (forced passes
/// are free), returning the exact score from `board.player`'s view. ProbCut (`search_probcut`,
/// `midgame.c:288-350`) fires only in midgame, in NWS context, at selective levels;
/// `probcut_level` caps recursive ProbCut at 2 (`LIMIT_RECURSIVE_PROBCUT`). `features` must be
/// kept consistent with `board` via [`update_features`]/[`undo_features`].
///
/// `in_midgame` (`depth < n_empties`) and `mover_is_eval_player` (`n_empties % 2 == 0`) are
/// caller-supplied: `depth - n_empties` is invariant from the root (moves decrement both, passes
/// neither), and the parity only flips on a real move.
#[allow(clippy::too_many_arguments)]
fn negamax(
    board: &Board,
    depth: i32,
    mut alpha: i32,
    beta: i32,
    selectivity: u32,
    probcut_level: u32,
    in_midgame: bool,
    mover_is_eval_player: bool,
    tt: &mut TranspositionTable,
    weights: &[EvalWeight],
    features: &mut [i32; 46],
) -> i32 {
    let n_empties = 64 - (board.player | board.opponent).count_ones() as i32;
    if n_empties == 0 {
        return final_score(board.player.count_ones() as i32, 0);
    }

    if depth == 0 {
        return eval_from_features(features, n_empties, weights);
    }

    let original_alpha = alpha;

    // Transposition cutoff (search_TC_NWS, search.c:1240-1256).
    if let Some(score) = tt.try_cutoff(board, depth, selectivity, alpha, beta) {
        return score;
    }

    // Stability cutoff (NWS_endgame, endgame.c:538-540), exact-solve only: the opponent's stable
    // discs cap the player's best score at SCORE_MAX - 2*s_opp; if that can't beat alpha, return.
    if !in_midgame && alpha >= crate::stability::NWS_STABILITY_THRESHOLD[n_empties as usize] {
        let s_opp = crate::stability::get_stability(board.opponent, board.player);
        let score = SCORE_MAX - 2 * s_opp as i32;
        if score <= alpha {
            return score;
        }
    }

    let moves = board.get_moves();
    if moves == 0 {
        let passed = board.pass();
        if passed.get_moves() == 0 {
            return final_score(board.player.count_ones() as i32, n_empties);
        }
        // A forced pass consumes no depth (midgame.c:632) and leaves n_empties unchanged, so
        // in_midgame and mover_is_eval_player pass through. In midgame, a pass swaps the eval
        // convention, so recompute features; in endgame, features are never read.
        if in_midgame {
            let mut passed_features = init_features(&passed, n_empties);
            return -negamax(
                &passed,
                depth,
                -beta,
                -alpha,
                selectivity,
                probcut_level,
                in_midgame,
                mover_is_eval_player,
                tt,
                weights,
                &mut passed_features,
            );
        }
        return -negamax(
            &passed,
            depth,
            -beta,
            -alpha,
            selectivity,
            probcut_level,
            in_midgame,
            mover_is_eval_player,
            tt,
            weights,
            features,
        );
    }

    // ProbCut (search_probcut, midgame.c:288-350): a shallow probe skips the full-depth search
    // when confident of a fail. `probcut_d = 0.25` (options.c:67); `RCD = 0.5` (midgame.c:24-26).
    if in_midgame && alpha + 1 == beta && selectivity < NO_SELECTIVITY && probcut_level < 2 {
        let probcut_depth = 2 * (depth / 4) + (depth & 1); // 2*floor(probcut_d*depth) + parity
        if probcut_depth >= 2 {
            // The probe searches the same board to a different depth: mover_is_eval_player
            // carries over, but in_midgame must be recomputed for probcut_depth.
            let probe_in_midgame = probcut_depth < n_empties;
            const RCD: f64 = 0.5;
            let t = SELECTIVITY_TABLE[selectivity as usize].t;
            let eval_score = eval_from_features(features, n_empties, weights);
            let probcut_error = (t * eval_sigma(n_empties, depth, probcut_depth) + RCD) as i32;
            let eval_error = (t
                * 0.5
                * (eval_sigma(n_empties, depth, 0) + eval_sigma(n_empties, depth, probcut_depth))
                + RCD) as i32;

            // Try a probable upper cut (beta cutoff): if shallow probe >= probcut_beta, return beta.
            let probcut_beta = beta + probcut_error;
            if eval_score >= beta - eval_error && probcut_beta < SCORE_MAX {
                let mut probe_feat = init_features(board, n_empties);
                let score = negamax(
                    board,
                    probcut_depth,
                    probcut_beta - 1,
                    probcut_beta,
                    selectivity,
                    probcut_level + 1,
                    probe_in_midgame,
                    mover_is_eval_player,
                    tt,
                    weights,
                    &mut probe_feat,
                );
                if score >= probcut_beta {
                    return beta;
                }
            }

            // Try a probable lower cut (alpha cutoff): if shallow probe <= probcut_alpha, return alpha.
            let probcut_alpha = alpha - probcut_error;
            if eval_score < alpha + eval_error && probcut_alpha > SCORE_MIN {
                let mut probe_feat = init_features(board, n_empties);
                let score = negamax(
                    board,
                    probcut_depth,
                    probcut_alpha,
                    probcut_alpha + 1,
                    selectivity,
                    probcut_level + 1,
                    probe_in_midgame,
                    mover_is_eval_player,
                    tt,
                    weights,
                    &mut probe_feat,
                );
                if score <= probcut_alpha {
                    return alpha;
                }
            }
        }
    }

    // Move ordering: ascending opponent mobility (the canonical Othello heuristic; doesn't
    // affect the minimax value). `sq` recovers `flipped = child.player ^ board.opponent`.
    // 34 is a safe upper bound on legal moves in any Othello position (observed max is ~33).
    let zero_board = Board {
        player: 0,
        opponent: 0,
    };
    let mut candidates = [(0i32, 0u32, zero_board); 34];
    let mut n = 0usize;
    let mut remaining_moves = moves;
    while remaining_moves != 0 {
        let x = remaining_moves.trailing_zeros();
        remaining_moves &= remaining_moves - 1;
        let child = board.play(x);
        let opp_mobility = child.get_moves().count_ones() as i32;
        candidates[n] = (opp_mobility, x, child);
        n += 1;
    }
    candidates[..n].sort_unstable_by_key(|&(score, _, _)| score);

    // Midgame: maintain features incrementally for depth-0 leaves. Endgame: eval is never
    // called, so skip the bookkeeping. A real move flips mover_is_eval_player.
    let child_mover_is_eval_player = !mover_is_eval_player;

    let mut best = SCORE_MIN - 1;
    for (i, &(_, x, child)) in candidates[..n].iter().enumerate() {
        let flipped = child.player ^ board.opponent;
        if in_midgame {
            update_features(features, x, flipped, mover_is_eval_player);
        }
        let score = if i == 0 {
            // First candidate: full-window search.
            -negamax(
                &child,
                depth - 1,
                -beta,
                -alpha,
                selectivity,
                probcut_level,
                in_midgame,
                child_mover_is_eval_player,
                tt,
                weights,
                features,
            )
        } else {
            // Subsequent candidates: null-window search first; re-search full-window only when
            // it improves alpha without proving a beta-cutoff.
            let nws = -negamax(
                &child,
                depth - 1,
                -alpha - 1,
                -alpha,
                selectivity,
                probcut_level,
                in_midgame,
                child_mover_is_eval_player,
                tt,
                weights,
                features,
            );
            if nws > alpha && nws < beta {
                -negamax(
                    &child,
                    depth - 1,
                    -beta,
                    -alpha,
                    selectivity,
                    probcut_level,
                    in_midgame,
                    child_mover_is_eval_player,
                    tt,
                    weights,
                    features,
                )
            } else {
                nws
            }
        };
        if in_midgame {
            undo_features(features, x, flipped, mover_is_eval_player);
        }
        if score > best {
            best = score;
        }
        if best > alpha {
            alpha = best;
        }
        if alpha >= beta {
            break;
        }
    }

    tt.store(board, depth, selectivity, original_alpha, beta, best);
    best
}

/// Evaluates `board` at Edax level `level` (`board.player` is the side to move), returning the
/// score from that side's view. Depth/selectivity come from [`depth_and_selectivity`]; levels
/// above 60 are rejected with [`UnsupportedLevel`].
pub fn solve(board: &Board, level: u32, weights: &[EvalWeight]) -> Result<i32, UnsupportedLevel> {
    if level > 60 {
        return Err(UnsupportedLevel(level));
    }
    let n_empties = 64 - (board.player | board.opponent).count_ones() as i32;
    let (depth, selectivity) = depth_and_selectivity(level, n_empties);
    // Feature state is initialised once at the root and maintained incrementally by the search.
    let mut tt = TranspositionTable::new(TT_SIZE_LOG2);
    let mut features = init_features(board, n_empties);
    Ok(negamax(
        board,
        depth,
        SCORE_MIN,
        SCORE_MAX,
        selectivity,
        0,
        depth < n_empties,
        n_empties % 2 == 0,
        &mut tt,
        weights,
        &mut features,
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::board::START;
    use crate::weights_transform::{N_PLIES, N_W};

    fn dummy_weights() -> Vec<EvalWeight> {
        let raw: Vec<i16> = (0..N_W * N_PLIES)
            .map(|i| ((i * 7 + 3) % 2001) as i16 - 1000)
            .collect();
        crate::weights::unpack(&raw, N_W, N_PLIES)
    }

    #[test]
    fn rejects_level_above_sixty() {
        let weights = dummy_weights();
        assert_eq!(solve(&START, 61, &weights), Err(UnsupportedLevel(61)));
    }

    #[test]
    fn accepts_levels_zero_through_sixty() {
        let weights = dummy_weights();
        assert!(solve(&START, 0, &weights).is_ok());
        assert!(solve(&START, 1, &weights).is_ok());
        assert!(solve(&START, 10, &weights).is_ok());
        // n_empties = 5: levels 11 and 60 both exact-solve, so ProbCut never fires.
        let small = Board {
            player: 0x007e_e4d2_aadc_be7c,
            opponent: 0x7e01_1b2d_5523_4180,
        };
        assert!(solve(&small, 11, &weights).is_ok());
        assert!(solve(&small, 60, &weights).is_ok());
    }

    /// Level 11+ sanity check: ProbCut-active solves stay in [-64, 64] and don't panic.
    #[test]
    fn level_eleven_plus_scores_are_in_range() {
        let weights = dummy_weights();
        // Level 12 at START: dep=12, sel=0 — ProbCut active. Level 0: dep=0, sel=5.
        let score_l0 = solve(&START, 0, &weights).unwrap();
        let score_l12 = solve(&START, 12, &weights).unwrap();
        assert!(
            (-64..=64).contains(&score_l0),
            "level 0 score {score_l0} out of range"
        );
        assert!(
            (-64..=64).contains(&score_l12),
            "level 12 score {score_l12} out of range"
        );
    }

    /// Spot-checks `depth_and_selectivity` against the C table (`search_global_init`,
    /// `search.c:161-346`); cases are (level, n_empties, expected_depth, expected_selectivity).
    #[test]
    fn depth_and_selectivity_matches_search_global_init() {
        let cases: &[(u32, i32, i32, u32)] = &[
            // level 0: dep = 0 always
            (0, 0, 0, 5),
            (0, 30, 0, 5),
            // level <= 10: dep = n_empties when n_empties <= 2*level, else dep = level
            (5, 8, 8, 5),    // 8 <= 10=2*5 → exact solve
            (5, 11, 5, 5),   // 11 > 10 → dep = level
            (10, 20, 20, 5), // 20 <= 20 → exact solve
            (10, 21, 10, 5), // 21 > 20 → dep = level
            // level <= 12
            (12, 21, 21, 5), // <= 21 → exact, sel = 5
            (12, 22, 22, 3), // 22..=24 → exact, sel = 3
            (12, 25, 12, 0), // > 24 → dep = level, sel = 0
            // level <= 18
            (15, 21, 21, 5), // <= 21 → exact, sel = 5
            (15, 22, 22, 3), // 22..=24 → exact, sel = 3
            (15, 25, 25, 1), // 25..=27 → exact, sel = 1
            (15, 28, 15, 0), // > 27 → dep = level, sel = 0
            // level <= 21
            (20, 24, 24, 5), // <= 24 → exact, sel = 5
            (20, 27, 27, 3), // 25..=27 → exact, sel = 3
            (20, 30, 30, 1), // 28..=30 → exact, sel = 1
            (20, 31, 20, 0), // > 30 → dep = level, sel = 0
            // level <= 24
            (23, 24, 24, 5), // <= 24 → exact, sel = 5
            (23, 27, 27, 4), // 25..=27 → exact, sel = 4
            (23, 30, 30, 2), // 28..=30 → exact, sel = 2
            (23, 33, 33, 0), // 31..=33 → exact, sel = 0
            (23, 34, 23, 0), // > 33 → dep = level, sel = 0
            // level < 30 (levels 28-29)
            (28, 27, 27, 5), // <= 27 → exact, sel = 5
            (28, 30, 30, 4), // 28..=30 → exact, sel = 4
            (28, 33, 33, 2), // 31..=33 → exact, sel = 2
            (28, 36, 36, 0), // 34..=36 → exact, sel = 0
            (28, 37, 28, 0), // > 36 → dep = level, sel = 0
            // level < 60 (level 40): thresholds are level±{6,3,0,3,6,9}
            (40, 34, 34, 5), // <= 40-6=34 → exact, sel = 5
            (40, 37, 37, 4), // <= 40-3=37 → exact, sel = 4
            (40, 40, 40, 3), // <= 40 → exact, sel = 3
            (40, 43, 43, 2), // <= 40+3=43 → exact, sel = 2
            (40, 46, 46, 1), // <= 40+6=46 → exact, sel = 1
            (40, 49, 49, 0), // <= 40+9=49 → exact, sel = 0
            (40, 50, 40, 0), // > 49 → dep = level, sel = 0
            // level 60: dep = n_empties, sel = 5 always
            (60, 0, 0, 5),
            (60, 30, 30, 5),
            (60, 60, 60, 5),
        ];
        for &(level, n_empties, exp_dep, exp_sel) in cases {
            assert_eq!(
                depth_and_selectivity(level, n_empties),
                (exp_dep, exp_sel),
                "depth_and_selectivity({level}, {n_empties})"
            );
        }
    }

    /// Any two levels reaching the exact-solve regime (n_empties <= 2 * level) never call
    /// eval_from_features, so they must agree regardless of weights.
    #[test]
    fn levels_use_real_per_level_depth() {
        let weights = dummy_weights();
        // 5 empties: levels 3, 5, and 10 all exact-solve and must agree.
        let board = Board {
            player: 0x007e_e4d2_aadc_be7c,
            opponent: 0x7e01_1b2d_5523_4180,
        };
        let exact = solve(&board, 10, &weights).unwrap();
        assert_eq!(solve(&board, 3, &weights), Ok(exact));
        assert_eq!(solve(&board, 5, &weights), Ok(exact));
        // Levels 0 and 2 don't reach the exact-solve regime here but must still succeed.
        assert!(solve(&board, 0, &weights).is_ok());
        assert!(solve(&board, 2, &weights).is_ok());
    }

    /// Alpha-beta correctness: an exact-solve score must be consistent with null-window probes
    /// on either side of it, independent of weights.
    #[test]
    fn exact_solve_score_is_consistent_with_null_window_probes() {
        let weights = dummy_weights();
        let mut board = START;
        let mut rng = 0xa5a5_1234_dead_beef_u64;
        let mut xorshift = || {
            rng ^= rng << 13;
            rng ^= rng >> 7;
            rng ^= rng << 17;
            rng
        };
        loop {
            let n_empties = 64 - (board.player | board.opponent).count_ones() as i32;
            if n_empties <= 10 {
                break;
            }
            let moves = board.get_moves();
            if moves == 0 {
                board = board.pass();
                continue;
            }
            let candidates: Vec<u32> = (0..64).filter(|&x| moves & (1u64 << x) != 0).collect();
            board = board.play(candidates[(xorshift() as usize) % candidates.len()]);
        }

        let n_empties = 64 - (board.player | board.opponent).count_ones() as i32;
        let in_midgame = 64 < n_empties; // depth=64 always exact-solves here (n_empties <= 10)
        let mover_is_eval_player = n_empties % 2 == 0;
        let mut tt = TranspositionTable::new(TT_SIZE_LOG2);
        let exact = negamax(
            &board,
            64,
            SCORE_MIN,
            SCORE_MAX,
            NO_SELECTIVITY,
            0,
            in_midgame,
            mover_is_eval_player,
            &mut tt,
            &weights,
            &mut init_features(&board, n_empties),
        );
        let mut tt = TranspositionTable::new(TT_SIZE_LOG2);
        let probe_low = negamax(
            &board,
            64,
            SCORE_MIN,
            exact,
            NO_SELECTIVITY,
            0,
            in_midgame,
            mover_is_eval_player,
            &mut tt,
            &weights,
            &mut init_features(&board, n_empties),
        );
        let mut tt = TranspositionTable::new(TT_SIZE_LOG2);
        let probe_high = negamax(
            &board,
            64,
            exact,
            SCORE_MAX,
            NO_SELECTIVITY,
            0,
            in_midgame,
            mover_is_eval_player,
            &mut tt,
            &weights,
            &mut init_features(&board, n_empties),
        );
        assert!(
            probe_low <= exact,
            "null-window probe below the true score returned {probe_low} > {exact}"
        );
        assert!(
            probe_high >= exact,
            "null-window probe above the true score returned {probe_high} < {exact}"
        );
    }

    /// Double pass with 46 empties left: game over before the board is full, the bug-prone case
    /// a full-board check alone would miss. Position found by independent random self-play.
    #[test]
    fn double_pass_before_board_full_ends_the_game_with_empties_awarded_to_the_leader() {
        let board = Board {
            player: 0x0446_241c_1c14_0200,
            opponent: 0x8000_0100_0000_0001,
        };
        assert_eq!(board.get_moves(), 0);
        assert_eq!(board.pass().get_moves(), 0);
        assert_eq!(64 - (board.player | board.opponent).count_ones(), 46);

        let weights = dummy_weights();
        // Game already over: the weights are never read, so their contents are irrelevant.
        assert_eq!(solve(&board, 10, &weights), Ok(58)); // independently hand-computed: 15+46 vs 3
    }

    /// The forced-pass fixture's game-over position (`n_empties == 0`): the other branch of the
    /// off-by-one-prone logic the test above exercises.
    #[test]
    fn real_game_over_fixture_scores_as_the_exact_disc_difference() {
        // "7fbfdfadd5abc5fe804020522a543a01-b": black (mover) 47 discs, white 17, 0 empties.
        let board = Board {
            player: 0x7fbf_dfad_d5ab_c5fe,
            opponent: 0x8040_2052_2a54_3a01,
        };
        assert_eq!(64 - (board.player | board.opponent).count_ones(), 0);
        assert!(board.get_moves() == 0 && board.pass().get_moves() == 0);

        let weights = dummy_weights();
        assert_eq!(solve(&board, 10, &weights), Ok(47 - 17));
    }

    /// The forced-pass fixture's ply-55 position (white to move, forced pass, 5 empties):
    /// exercises pass detection deep inside the search, not just at the root.
    #[test]
    fn solves_exactly_through_a_real_mid_search_forced_pass() {
        // "7e011b2d55234180007ee4d2aadcbe7c-w": black=7e011b2d55234180, white=007ee4d2aadcbe7c,
        // white to move.
        let black: u64 = 0x7e01_1b2d_5523_4180;
        let white: u64 = 0x007e_e4d2_aadc_be7c;
        let board = Board {
            player: white,
            opponent: black,
        }; // "-w": white is the mover
        assert_eq!(64 - (board.player | board.opponent).count_ones(), 5);
        assert_eq!(
            board.get_moves(),
            0,
            "this position is white's forced pass in the real game"
        );

        let weights = dummy_weights();
        let result = solve(&board, 10, &weights);
        assert!(result.is_ok());
        let score = result.unwrap();
        assert!((-64..=64).contains(&score));
    }

    /// Verifies `SELECTIVITY_TABLE` against `search.c:104-111` and that `NO_SELECTIVITY`
    /// indexes the last (full-width) entry.
    #[test]
    fn selectivity_table_matches_search_c() {
        // (t, level, percent) from search.c:104-111
        let expected: [(f64, u32, u32); 6] = [
            (1.1, 0, 73),
            (1.5, 1, 87),
            (2.0, 2, 95),
            (2.6, 3, 98),
            (3.3, 4, 99),
            (999.0, 5, 100),
        ];
        for (i, &(t, level, percent)) in expected.iter().enumerate() {
            assert_eq!(SELECTIVITY_TABLE[i].t, t, "entry {i} t");
            assert_eq!(SELECTIVITY_TABLE[i].level, level, "entry {i} level");
            assert_eq!(SELECTIVITY_TABLE[i].percent, percent, "entry {i} percent");
        }
        assert_eq!(
            SELECTIVITY_TABLE[NO_SELECTIVITY as usize].level,
            NO_SELECTIVITY
        );
        assert_eq!(SELECTIVITY_TABLE[NO_SELECTIVITY as usize].percent, 100);
    }

    #[test]
    fn final_score_awards_empties_to_the_leader_and_splits_on_tie() {
        assert_eq!(final_score(32, 0), 0); // tied on a full board
        assert_eq!(final_score(40, 0), 16); // 40-24 on a full board
        assert_eq!(final_score(20, 0), -24); // 20-44 on a full board
                                             // Leading 33-21 with 10 empties left, game over (double pass): leader gets all 10.
        assert_eq!(final_score(33, 10), 22); // (33+10) - 21 = 22
                                             // Tied 27-27 with 10 empties, game over: empties don't break the tie.
        assert_eq!(final_score(27, 10), 0);
        // Behind 27-33 with 4 empties, game over: opponent gets all 4.
        assert_eq!(final_score(27, 4), -10); // 27 - (33+4) = -10
    }
}
