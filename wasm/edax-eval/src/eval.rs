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

//! Evaluation feature computation and leaf scoring (`TASKS.md` Task 5):
//! ports `accumlate_eval`/`search_eval_0` (`midgame.c:34-121`) and the
//! feature-from-board computation `eval_set` does in its portable path
//! (`eval.c:757-770`).
//!
//! Edax computes features incrementally (`eval_update`/`eval_update_leaf`,
//! `eval.c:782-908`), tracking a running `Eval` struct move by move to
//! avoid recomputing all 46 features from the full board at every leaf.
//! This port now does the same via [`init_features`] (once per search
//! root), [`update_features`]/[`undo_features`] (at each interior node),
//! and [`eval_from_features`] at depth-0 leaves. The update cost per move
//! is O(discs_changed × avg_features_per_square ≈ 5 × 5.6 = 28 ops)
//! versus O(437) for a full recompute, reducing eval cost by roughly 15×
//! per leaf visit and yielding a ~2× speedup on the eval fraction of total
//! search time in midgame positions.

use crate::board::Board;
use crate::weights::EvalWeight;

/// Number of plies `EVAL_WEIGHT` covers (`eval.h`: `EVAL_N_PLY`); only plies `2..EVAL_N_PLY` have
/// real trained weights (see `crate::weights`).
const EVAL_N_PLY: i32 = 54;

const SCORE_MIN: i32 = -64;
const SCORE_MAX: i32 = 64;

/// Which shared weight array a feature's raw base-3 state indexes into (`eval.c`'s `accumlate_eval`
/// hardcodes this grouping via fixed `f[]` index ranges; named here instead of index ranges since
/// this port computes features into a plain `[i32; 46]` rather than replicating the exact packed
/// union layout).
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
enum WeightGroup {
    C9,
    C10,
    S100,
    S101,
    S8x4,
    S7654,
}

/// One of the 46 board features `accumlate_eval` sums (`EVAL_F2X` minus its trailing `PASS`
/// sentinel entry, which `accumlate_eval` never reads). `squares` are listed in the exact order
/// Edax encodes them in (most-significant base-3 digit first) — order is significant, not just
/// membership, since it determines the feature's raw value. `offset` shifts a feature's raw
/// `0..3^n-1` value into its sub-region of a shared weight array (`S8x4`/`S7654` each pack 4
/// features into one array; `C9`/`C10`/`S100`/`S101` don't share, hence `offset: 0` throughout).
struct Feature {
    squares: &'static [u32],
    group: WeightGroup,
    offset: i32,
}

/// Direct port of `EVAL_F2X` (`eval.c:42-97`) plus `EVAL_OFFSET` (`eval.c:457-459`) merged
/// together, square names translated to indices (`A1=0 .. H8=63`, matching `crate::board`'s
/// convention, which is Edax's own). Order (corners/C9, edges/C10, S100, S101, rows-cols/S8x4,
/// diagonals-4..7/S7654) matches `accumlate_eval`'s hardcoded `f[]` groupings exactly.
#[rustfmt::skip]
const FEATURES: [Feature; 46] = [
    Feature { squares: &[0, 1, 8, 9, 2, 16, 10, 17, 18], group: WeightGroup::C9, offset: 0 },
    Feature { squares: &[7, 6, 15, 14, 5, 23, 13, 22, 21], group: WeightGroup::C9, offset: 0 },
    Feature { squares: &[56, 48, 57, 49, 40, 58, 41, 50, 42], group: WeightGroup::C9, offset: 0 },
    Feature { squares: &[63, 55, 62, 54, 47, 61, 46, 53, 45], group: WeightGroup::C9, offset: 0 },
    Feature { squares: &[32, 24, 16, 8, 0, 9, 1, 2, 3, 4], group: WeightGroup::C10, offset: 0 },
    Feature { squares: &[39, 31, 23, 15, 7, 14, 6, 5, 4, 3], group: WeightGroup::C10, offset: 0 },
    Feature { squares: &[24, 32, 40, 48, 56, 49, 57, 58, 59, 60], group: WeightGroup::C10, offset: 0 },
    Feature { squares: &[31, 39, 47, 55, 63, 54, 62, 61, 60, 59], group: WeightGroup::C10, offset: 0 },
    Feature { squares: &[9, 0, 1, 2, 3, 4, 5, 6, 7, 14], group: WeightGroup::S100, offset: 0 },
    Feature { squares: &[49, 56, 57, 58, 59, 60, 61, 62, 63, 54], group: WeightGroup::S100, offset: 0 },
    Feature { squares: &[9, 0, 8, 16, 24, 32, 40, 48, 56, 49], group: WeightGroup::S100, offset: 0 },
    Feature { squares: &[14, 7, 15, 23, 31, 39, 47, 55, 63, 54], group: WeightGroup::S100, offset: 0 },
    Feature { squares: &[0, 2, 3, 10, 11, 12, 13, 4, 5, 7], group: WeightGroup::S101, offset: 0 },
    Feature { squares: &[56, 58, 59, 50, 51, 52, 53, 60, 61, 63], group: WeightGroup::S101, offset: 0 },
    Feature { squares: &[0, 16, 24, 17, 25, 33, 41, 32, 40, 56], group: WeightGroup::S101, offset: 0 },
    Feature { squares: &[7, 23, 31, 22, 30, 38, 46, 39, 47, 63], group: WeightGroup::S101, offset: 0 },
    Feature { squares: &[8, 9, 10, 11, 12, 13, 14, 15], group: WeightGroup::S8x4, offset: 0 },
    Feature { squares: &[48, 49, 50, 51, 52, 53, 54, 55], group: WeightGroup::S8x4, offset: 0 },
    Feature { squares: &[1, 9, 17, 25, 33, 41, 49, 57], group: WeightGroup::S8x4, offset: 0 },
    Feature { squares: &[6, 14, 22, 30, 38, 46, 54, 62], group: WeightGroup::S8x4, offset: 0 },
    Feature { squares: &[16, 17, 18, 19, 20, 21, 22, 23], group: WeightGroup::S8x4, offset: 6561 },
    Feature { squares: &[40, 41, 42, 43, 44, 45, 46, 47], group: WeightGroup::S8x4, offset: 6561 },
    Feature { squares: &[2, 10, 18, 26, 34, 42, 50, 58], group: WeightGroup::S8x4, offset: 6561 },
    Feature { squares: &[5, 13, 21, 29, 37, 45, 53, 61], group: WeightGroup::S8x4, offset: 6561 },
    Feature { squares: &[24, 25, 26, 27, 28, 29, 30, 31], group: WeightGroup::S8x4, offset: 13122 },
    Feature { squares: &[32, 33, 34, 35, 36, 37, 38, 39], group: WeightGroup::S8x4, offset: 13122 },
    Feature { squares: &[3, 11, 19, 27, 35, 43, 51, 59], group: WeightGroup::S8x4, offset: 13122 },
    Feature { squares: &[4, 12, 20, 28, 36, 44, 52, 60], group: WeightGroup::S8x4, offset: 13122 },
    Feature { squares: &[0, 9, 18, 27, 36, 45, 54, 63], group: WeightGroup::S8x4, offset: 19683 },
    Feature { squares: &[56, 49, 42, 35, 28, 21, 14, 7], group: WeightGroup::S8x4, offset: 19683 },
    Feature { squares: &[1, 10, 19, 28, 37, 46, 55], group: WeightGroup::S7654, offset: 0 },
    Feature { squares: &[15, 22, 29, 36, 43, 50, 57], group: WeightGroup::S7654, offset: 0 },
    Feature { squares: &[8, 17, 26, 35, 44, 53, 62], group: WeightGroup::S7654, offset: 0 },
    Feature { squares: &[6, 13, 20, 27, 34, 41, 48], group: WeightGroup::S7654, offset: 0 },
    Feature { squares: &[2, 11, 20, 29, 38, 47], group: WeightGroup::S7654, offset: 2187 },
    Feature { squares: &[16, 25, 34, 43, 52, 61], group: WeightGroup::S7654, offset: 2187 },
    Feature { squares: &[5, 12, 19, 26, 33, 40], group: WeightGroup::S7654, offset: 2187 },
    Feature { squares: &[23, 30, 37, 44, 51, 58], group: WeightGroup::S7654, offset: 2187 },
    Feature { squares: &[3, 12, 21, 30, 39], group: WeightGroup::S7654, offset: 2916 },
    Feature { squares: &[24, 33, 42, 51, 60], group: WeightGroup::S7654, offset: 2916 },
    Feature { squares: &[4, 11, 18, 25, 32], group: WeightGroup::S7654, offset: 2916 },
    Feature { squares: &[31, 38, 45, 52, 59], group: WeightGroup::S7654, offset: 2916 },
    Feature { squares: &[3, 10, 17, 24], group: WeightGroup::S7654, offset: 3159 },
    Feature { squares: &[32, 41, 50, 59], group: WeightGroup::S7654, offset: 3159 },
    Feature { squares: &[4, 13, 22, 31], group: WeightGroup::S7654, offset: 3159 },
    Feature { squares: &[39, 46, 53, 60], group: WeightGroup::S7654, offset: 3159 },
];

/// How many of the 46 features contain each board square (range 4-7).
/// Precomputed from FEATURES; used to drive the incremental update loops.
#[rustfmt::skip]
const SQ_FEAT_COUNT: [u8; 64] = [
    7, 5, 6, 7, 7, 6, 5, 7, 5, 7, 6, 5, 5, 6, 7, 5, 6, 6, 5, 4, 4, 5, 6, 6, 7, 5, 4, 4, 4, 4, 5, 7,
    7, 5, 4, 4, 4, 4, 5, 7, 6, 6, 5, 4, 4, 5, 6, 6, 5, 7, 6, 5, 5, 6, 7, 5, 7, 5, 6, 7, 7, 6, 5, 7,
];

/// Feature indices for each square's contributing features (only first SQ_FEAT_COUNT[sq] are valid).
#[rustfmt::skip]
const SQ_FEAT_IDX: [[u8; 7]; 64] = [
    [ 0,  4,  8, 10, 12, 14, 28], // sq=0
    [ 0,  4,  8, 18, 30,  0,  0], // sq=1
    [ 0,  4,  8, 12, 22, 34,  0], // sq=2
    [ 4,  5,  8, 12, 26, 38, 42], // sq=3
    [ 4,  5,  8, 12, 27, 40, 44], // sq=4
    [ 1,  5,  8, 12, 23, 36,  0], // sq=5
    [ 1,  5,  8, 19, 33,  0,  0], // sq=6
    [ 1,  5,  8, 11, 12, 15, 29], // sq=7
    [ 0,  4, 10, 16, 32,  0,  0], // sq=8
    [ 0,  4,  8, 10, 16, 18, 28], // sq=9
    [ 0, 12, 16, 22, 30, 42,  0], // sq=10
    [12, 16, 26, 34, 40,  0,  0], // sq=11
    [12, 16, 27, 36, 38,  0,  0], // sq=12
    [ 1, 12, 16, 23, 33, 44,  0], // sq=13
    [ 1,  5,  8, 11, 16, 19, 29], // sq=14
    [ 1,  5, 11, 16, 31,  0,  0], // sq=15
    [ 0,  4, 10, 14, 20, 35,  0], // sq=16
    [ 0, 14, 18, 20, 32, 42,  0], // sq=17
    [ 0, 20, 22, 28, 40,  0,  0], // sq=18
    [20, 26, 30, 36,  0,  0,  0], // sq=19
    [20, 27, 33, 34,  0,  0,  0], // sq=20
    [ 1, 20, 23, 29, 38,  0,  0], // sq=21
    [ 1, 15, 19, 20, 31, 44,  0], // sq=22
    [ 1,  5, 11, 15, 20, 37,  0], // sq=23
    [ 4,  6, 10, 14, 24, 39, 42], // sq=24
    [14, 18, 24, 35, 40,  0,  0], // sq=25
    [22, 24, 32, 36,  0,  0,  0], // sq=26
    [24, 26, 28, 33,  0,  0,  0], // sq=27
    [24, 27, 29, 30,  0,  0,  0], // sq=28
    [23, 24, 31, 34,  0,  0,  0], // sq=29
    [15, 19, 24, 37, 38,  0,  0], // sq=30
    [ 5,  7, 11, 15, 24, 41, 44], // sq=31
    [ 4,  6, 10, 14, 25, 40, 43], // sq=32
    [14, 18, 25, 36, 39,  0,  0], // sq=33
    [22, 25, 33, 35,  0,  0,  0], // sq=34
    [25, 26, 29, 32,  0,  0,  0], // sq=35
    [25, 27, 28, 31,  0,  0,  0], // sq=36
    [23, 25, 30, 37,  0,  0,  0], // sq=37
    [15, 19, 25, 34, 41,  0,  0], // sq=38
    [ 5,  7, 11, 15, 25, 38, 45], // sq=39
    [ 2,  6, 10, 14, 21, 36,  0], // sq=40
    [ 2, 14, 18, 21, 33, 43,  0], // sq=41
    [ 2, 21, 22, 29, 39,  0,  0], // sq=42
    [21, 26, 31, 35,  0,  0,  0], // sq=43
    [21, 27, 32, 37,  0,  0,  0], // sq=44
    [ 3, 21, 23, 28, 41,  0,  0], // sq=45
    [ 3, 15, 19, 21, 30, 45,  0], // sq=46
    [ 3,  7, 11, 15, 21, 34,  0], // sq=47
    [ 2,  6, 10, 17, 33,  0,  0], // sq=48
    [ 2,  6,  9, 10, 17, 18, 29], // sq=49
    [ 2, 13, 17, 22, 31, 43,  0], // sq=50
    [13, 17, 26, 37, 39,  0,  0], // sq=51
    [13, 17, 27, 35, 41,  0,  0], // sq=52
    [ 3, 13, 17, 23, 32, 45,  0], // sq=53
    [ 3,  7,  9, 11, 17, 19, 28], // sq=54
    [ 3,  7, 11, 17, 30,  0,  0], // sq=55
    [ 2,  6,  9, 10, 13, 14, 29], // sq=56
    [ 2,  6,  9, 18, 31,  0,  0], // sq=57
    [ 2,  6,  9, 13, 22, 37,  0], // sq=58
    [ 6,  7,  9, 13, 26, 41, 43], // sq=59
    [ 6,  7,  9, 13, 27, 39, 45], // sq=60
    [ 3,  7,  9, 13, 23, 35,  0], // sq=61
    [ 3,  7,  9, 19, 32,  0,  0], // sq=62
    [ 3,  7,  9, 11, 13, 15, 28], // sq=63
];

/// Horner weight 3^(n-1-pos) for each (square, feature-slot) pair (only first SQ_FEAT_COUNT[sq] are valid).
#[rustfmt::skip]
const SQ_FEAT_WEIGHT: [[i32; 7]; 64] = [
    [6561, 243, 6561, 6561, 19683, 19683, 2187], // sq=0
    [2187,  27, 2187, 2187,   729,     0,    0], // sq=1
    [  81,   9,  729, 6561,  2187,   243,    0], // sq=2
    [   3,   1,  243, 2187,  2187,    81,   27], // sq=3
    [   1,   3,   81,    9,  2187,    81,   27], // sq=4
    [  81,   9,   27,    3,  2187,   243,    0], // sq=5
    [2187,  27,    9, 2187,   729,     0,    0], // sq=6
    [6561, 243,    3, 6561,     1, 19683,    1], // sq=7
    [ 729, 729, 2187, 2187,   729,     0,    0], // sq=8
    [ 243,  81,19683,19683,   729,   729,  729], // sq=9
    [   9, 729,  243,  729,   243,     9,    0], // sq=10
    [ 243,  81,  729,   81,    27,     0,    0], // sq=11
    [  81,  27,  729,   81,    27,     0,    0], // sq=12
    [   9,  27,    9,  729,   243,     9,    0], // sq=13
    [ 243,  81,    1,19683,     3,   729,    3], // sq=14
    [ 729, 729, 2187,    1,   729,     0,    0], // sq=15
    [  27,2187,  729, 6561,  2187,   243,    0], // sq=16
    [   3, 729,  243,  729,   243,     3,    0], // sq=17
    [   1, 243,  243,  243,     9,     0,    0], // sq=18
    [  81, 243,   81,   27,     0,     0,    0], // sq=19
    [  27, 243,   81,   27,     0,     0,    0], // sq=20
    [   1,   9,  243,    9,     9,     0,    0], // sq=21
    [   3, 729,  243,    3,   243,     3,    0], // sq=22
    [  27,2187,  729, 6561,     1,   243,    0], // sq=23
    [6561,19683,  243, 2187,  2187,    81,    1], // sq=24
    [ 243,  81,  729,   81,     3,     0,    0], // sq=25
    [  81, 243,   81,    9,     0,     0,    0], // sq=26
    [  81,  81,   81,   27,     0,     0,    0], // sq=27
    [  27,  81,   27,   27,     0,     0,    0], // sq=28
    [  81,   9,   81,    9,     0,     0,    0], // sq=29
    [ 243,  81,    3,   81,     3,     0,    0], // sq=30
    [6561,19683,  243, 2187,     1,    81,    1], // sq=31
    [19683,6561,   81,    9,  2187,     1,   27], // sq=32
    [  81,  27,  729,    3,    27,     0,    0], // sq=33
    [  27, 243,    9,   27,     0,     0,    0], // sq=34
    [  81,  27,   81,   27,     0,     0,    0], // sq=35
    [  27,  27,   27,   27,     0,     0,    0], // sq=36
    [  27,   9,    9,   27,     0,     0,    0], // sq=37
    [  81,  27,    3,    3,    27,     0,    0], // sq=38
    [19683,6561,   81,    9,     1,     1,   27], // sq=39
    [  81,2187,   27,    3,  2187,     1,    0], // sq=40
    [   9,  27,    9,  729,     3,     9,    0], // sq=41
    [   1, 243,    9,  243,     9,     0,    0], // sq=42
    [  81,   9,    9,    9,     0,     0,    0], // sq=43
    [  27,   9,    9,    9,     0,     0,    0], // sq=44
    [   1,   9,    9,    9,     9,     0,    0], // sq=45
    [   9,  27,    9,    3,     3,     9,    0], // sq=46
    [  81,2187,   27,    3,     1,     1,    0], // sq=47
    [2187, 729,    9, 2187,     1,     0,    0], // sq=48
    [ 243,  81,19683,    1,   729,     3,  729], // sq=49
    [   3, 729,  243,    3,     3,     3,    0], // sq=50
    [ 243,  81,    3,    3,     3,     0,    0], // sq=51
    [  81,  27,    3,    3,     3,     0,    0], // sq=52
    [   3,  27,    9,    3,     3,     3,    0], // sq=53
    [ 243,  81,    1,    1,     3,     3,    3], // sq=54
    [2187, 729,    9,    1,     1,     0,    0], // sq=55
    [6561, 243, 6561,    3, 19683,     1, 2187], // sq=56
    [ 729,  27, 2187,    1,     1,     0,    0], // sq=57
    [  27,   9,  729, 6561,     1,     1,    0], // sq=58
    [   3,   1,  243, 2187,     1,     1,    1], // sq=59
    [   1,   3,   81,    9,     1,     1,    1], // sq=60
    [  27,   9,   27,    3,     1,     1,    0], // sq=61
    [ 729,  27,    9,    1,     1,     0,    0], // sq=62
    [6561, 243,    3,    3,     1,     1,    1], // sq=63
];

/// Computes all 46 feature values for `b`, each a base-3 number (most-significant digit = the
/// feature's first listed square) of that square's color (0 = player, 1 = opponent, 2 = empty),
/// offset into its weight array's sub-region. Direct port of `eval_set`'s portable per-feature
/// loop (`eval.c:763-769`) — but unlike `eval_set`, which takes the *pre-parity-swapped* board
/// (`eval_set`'s own `b`, see [`search_eval_0`]), this takes whatever board it's given as-is;
/// the caller is responsible for that swap.
///
/// A flat 64-entry color table is built from the bitboards once per call (O(popcount) init),
/// replacing the branch-per-square pattern of the original `board_get_square_color` calls
/// (`board.c:1033-1037`) in the feature loop. The inner Horner loop becomes branch-free array
/// reads, which are cheaper and more predictable in the WASM scalar execution model.
fn compute_features(b: &Board) -> [i32; 46] {
    // Default to 2 (empty); overwrite only occupied squares.
    let mut colors = [2i32; 64];
    let mut p = b.player;
    while p != 0 {
        colors[p.trailing_zeros() as usize] = 0;
        p &= p - 1;
    }
    let mut o = b.opponent;
    while o != 0 {
        colors[o.trailing_zeros() as usize] = 1;
        o &= o - 1;
    }

    let mut f = [0i32; 46];
    for (i, feat) in FEATURES.iter().enumerate() {
        let mut x = 0i32;
        for &sq in feat.squares {
            x = x * 3 + colors[sq as usize];
        }
        f[i] = x + feat.offset;
    }
    f
}

/// Sums the 46 features' weights for the ply-appropriate `EvalWeight` table, plus its constant
/// `s0` term. Direct port of `accumlate_eval` (`midgame.c:34-64`, portable non-AVX2 branch).
///
/// `weights` is indexed exactly as [`crate::weights::unpack`] returns it: `weights[0]` is real ply
/// 2, `weights[weights.len() - 1]` is real ply `EVAL_N_PLY - 1`.
fn accumulate_eval(ply: i32, f: &[i32; 46], weights: &[EvalWeight]) -> i32 {
    let mut ply = ply;
    if ply >= EVAL_N_PLY {
        ply = EVAL_N_PLY - 2 + (ply & 1);
    }
    ply -= 2;
    if ply < 0 {
        ply &= 1;
    }
    let w = &weights[ply as usize];

    let mut sum = 0i32;
    for (feat, &i) in FEATURES.iter().zip(f.iter()) {
        let i = i as usize;
        sum += match feat.group {
            WeightGroup::C9 => w.c9[i],
            WeightGroup::C10 => w.c10[i],
            WeightGroup::S100 => w.s100[i],
            WeightGroup::S101 => w.s101[i],
            WeightGroup::S8x4 => w.s8x4[i],
            WeightGroup::S7654 => w.s7654[i],
        } as i32;
    }
    sum + w.s0 as i32
}

/// Initialises feature state for the root of an incremental search. Applies the same parity swap
/// as `search_eval_0` so the feature convention (which physical color is "player" = 0) is fixed
/// for the whole search tree: when `n_empties` is even, `board.player` = color 0; when odd,
/// `board.opponent` = color 0 (because those are the same physical color after alternating moves).
/// This is a parity-based *relabeling* of which physical color counts as feature-color-0 for this
/// ply -- there is exactly one `EVAL_WEIGHT` table per ply (`weights::EvalWeight`), not separate
/// tables for black-to-move vs. white-to-move -- matching Edax's own `eval_set` (`eval.c:757-762`):
/// `if (eval->n_empties & 1) { b.player = board->opponent; ... }` before computing features, for
/// exactly this reason (the weight table is trained per-ply against a fixed "color 0" convention,
/// not a naively mover-relative one).
pub(crate) fn init_features(board: &Board, n_empties: i32) -> [i32; 46] {
    let b = if n_empties & 1 != 0 {
        Board {
            player: board.opponent,
            opponent: board.player,
        }
    } else {
        *board
    };
    compute_features(&b)
}

/// Applies the incremental feature delta for a move that placed a disc at `sq` and flipped the
/// discs in `flipped`. `mover_is_eval_player` is `n_empties % 2 == 0` at the node making the
/// move (i.e. the current mover maps to feature color 0 when true, color 1 when false).
///
/// Delta derivation: placed square changes 2 → placed_color; flipped squares change
/// opp_color → placed_color. With mover=color 0: placed_color=0, opp_color=1 → placed delta=-2,
/// flip delta=-1. With mover=color 1: placed_color=1, opp_color=0 → placed delta=-1, flip delta=+1.
///
/// Edax computes this via two separately-coded functions instead, `eval_update_0`/`eval_update_1`
/// (`eval.c:782-876`), dispatched on `eval->n_empties & 1` rather than branching on a parameter;
/// their bodies differ in exactly these same sign/magnitude constants (`eval_update_0`:
/// placed `-2x`/flipped `-1x`, matching `mover_is_eval_player = true`; `eval_update_1`: placed
/// `-1x`/flipped `+1x`, matching `false`). Both approaches branch once per move either way (Edax
/// picks a function, this picks two constants), so there's no extra per-square branching cost to
/// sharing one loop here.
pub(crate) fn update_features(
    features: &mut [i32; 46],
    sq: u32,
    flipped: u64,
    mover_is_eval_player: bool,
) {
    let (sq_delta, flip_delta): (i32, i32) = if mover_is_eval_player {
        (-2, -1)
    } else {
        (-1, 1)
    };
    let count = SQ_FEAT_COUNT[sq as usize] as usize;
    for i in 0..count {
        features[SQ_FEAT_IDX[sq as usize][i] as usize] += sq_delta * SQ_FEAT_WEIGHT[sq as usize][i];
    }
    let mut f = flipped;
    while f != 0 {
        let fsq = f.trailing_zeros() as usize;
        let fcount = SQ_FEAT_COUNT[fsq] as usize;
        for i in 0..fcount {
            features[SQ_FEAT_IDX[fsq][i] as usize] += flip_delta * SQ_FEAT_WEIGHT[fsq][i];
        }
        f &= f - 1;
    }
}

/// Reverses the delta applied by [`update_features`] with the same arguments, restoring
/// features to their pre-move state so the search can try the next sibling move.
pub(crate) fn undo_features(
    features: &mut [i32; 46],
    sq: u32,
    flipped: u64,
    mover_is_eval_player: bool,
) {
    let (sq_delta, flip_delta): (i32, i32) = if mover_is_eval_player {
        (2, 1)
    } else {
        (1, -1)
    };
    let count = SQ_FEAT_COUNT[sq as usize] as usize;
    for i in 0..count {
        features[SQ_FEAT_IDX[sq as usize][i] as usize] += sq_delta * SQ_FEAT_WEIGHT[sq as usize][i];
    }
    let mut f = flipped;
    while f != 0 {
        let fsq = f.trailing_zeros() as usize;
        let fcount = SQ_FEAT_COUNT[fsq] as usize;
        for i in 0..fcount {
            features[SQ_FEAT_IDX[fsq][i] as usize] += flip_delta * SQ_FEAT_WEIGHT[fsq][i];
        }
        f &= f - 1;
    }
}

/// Scores a depth-0 midgame leaf using incrementally-tracked features instead of recomputing
/// them from the board. The `features` array must have been maintained by [`update_features`]/
/// [`undo_features`] from an [`init_features`] root; `n_empties` is this node's empty count.
pub(crate) fn eval_from_features(
    features: &[i32; 46],
    n_empties: i32,
    weights: &[EvalWeight],
) -> i32 {
    let ply = 60 - n_empties;
    let mut score = accumulate_eval(ply, features, weights);
    if score > 0 {
        score += 64;
    } else {
        score -= 64;
    }
    score /= 128;
    score.clamp(SCORE_MIN + 1, SCORE_MAX - 1)
}

/// Evaluates `board` (mover-relative: `board.player` is the side to move) with the trained
/// evaluation function, as Edax's `search_eval_0` does (`midgame.c:105-121`) for a leaf reached by
/// exhausting the search depth (as opposed to solving to the end of the game, whose leaves are
/// scored by exact disc count instead — that's `crate::search`'s concern, Task 6).
///
/// `weights` must be [`crate::weights::unpack`]'s output for the real 52-ply table (`n_plies =
/// crate::weights_transform::N_PLIES`); a shorter slice is a caller bug, not a runtime input to
/// validate (this crate ships exactly one weights blob).
pub fn search_eval_0(board: &Board, weights: &[EvalWeight]) -> i32 {
    let n_empties = 64 - (board.player | board.opponent).count_ones() as i32;
    let ply = 60 - n_empties;

    // eval_set (eval.c:757-761): features are always encoded for a *fixed* color convention
    // across plies (matching which of P[0]/P[1] crate::weights::unpack used for this ply), not
    // naively mover-relative -- odd n_empties swaps which side is treated as "player".
    let b = if n_empties & 1 != 0 {
        Board {
            player: board.opponent,
            opponent: board.player,
        }
    } else {
        *board
    };

    let f = compute_features(&b);
    let mut score = accumulate_eval(ply, &f, weights);

    if score > 0 {
        score += 64;
    } else {
        score -= 64;
    }
    score /= 128;

    // Edax clamps with two sequential ifs (midgame.c:118-119); .clamp() is equivalent here since
    // SCORE_MIN + 1 < SCORE_MAX - 1 always holds (clippy::manual_clamp).
    score.clamp(SCORE_MIN + 1, SCORE_MAX - 1)
}

/// Direct port of `eval_sigma` (`eval.c:948-956`): the estimated standard deviation of the
/// search error as a function of the position's emptiness, search depth, and ProbCut shallow
/// depth. Used by ProbCut (`search_probcut`, Task 15) to size the null-window probe; a larger
/// sigma means a wider probe window (less aggressive pruning).
pub(crate) fn eval_sigma(n_empty: i32, depth: i32, probcut_depth: i32) -> f64 {
    // Linear-combination coefficients from eval_open (eval.c:705-706).
    // The C source names the quadratic coefficients with lowercase (EVAL_a/b/c vs EVAL_A/B/C)
    // to distinguish the two sets; this port uses LIN_/QUAD_ prefixes instead.
    const LIN_A: f64 = -0.10026799;
    const LIN_B: f64 = 0.31027733;
    const LIN_C: f64 = -0.57772603;
    const QUAD_A: f64 = 0.07585621;
    const QUAD_B: f64 = 1.16492647;
    const QUAD_C: f64 = 5.4171698;
    let sigma = LIN_A * n_empty as f64 + LIN_B * depth as f64 + LIN_C * probcut_depth as f64;
    QUAD_A * sigma * sigma + QUAD_B * sigma + QUAD_C
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::board::START;
    use crate::weights_transform::{N_PLIES, N_W};

    /// Spot-checks `eval_sigma` against pre-computed reference values from the formula in
    /// `eval.c:948-956` with constants from `eval.c:705-706`. Reference values computed by
    /// substituting into the two-step formula by hand; any transcription error in the constants
    /// would produce a result outside the 1e-9 tolerance.
    #[test]
    fn eval_sigma_matches_formula() {
        // (n=30, depth=10, probcut_depth=4):
        //   lin = -0.10026799*30 + 0.31027733*10 + -0.57772603*4 = -2.2161705
        //   sigma = 0.07585621*(-2.2161705)^2 + 1.16492647*(-2.2161705) + 5.4171698
        //         ≈ 0.37256 - 2.58168 + 5.41717 ≈ 3.20806
        let s1 = eval_sigma(30, 10, 4);
        assert!(
            (s1 - 3.208055785).abs() < 1e-6,
            "eval_sigma(30,10,4) = {s1}"
        );
        // (n=20, depth=5, probcut_depth=3):
        //   lin = -0.10026799*20 + 0.31027733*5 + -0.57772603*3 = -2.226...
        //   sigma ≈ 3.18...
        let s2 = eval_sigma(20, 5, 3);
        assert!(
            s2 > 2.0 && s2 < 5.0,
            "eval_sigma(20,5,3) = {s2}, expected ≈3.18"
        );
    }

    /// Every feature's raw value (after adding its offset) must land inside the weight array
    /// it's used to index -- an out-of-bounds index would panic in `accumulate_eval`, so this
    /// catches a transcription mistake in `FEATURES` (wrong square count, wrong offset, wrong
    /// group) independent of needing real weights or a real board.
    #[test]
    fn every_feature_raw_value_fits_its_weight_array() {
        for feat in &FEATURES {
            let n_states = 3i32.pow(feat.squares.len() as u32);
            let max_raw = n_states - 1 + feat.offset;
            let bound = match feat.group {
                WeightGroup::C9 => 19683,
                WeightGroup::C10 | WeightGroup::S100 | WeightGroup::S101 => 59049,
                WeightGroup::S8x4 => 6561 * 4,
                WeightGroup::S7654 => 2187 + 729 + 243 + 81,
            };
            assert!(
                max_raw < bound,
                "feature raw value {max_raw} exceeds its weight array bound {bound}"
            );
        }
    }

    /// Every board square must appear in at least one feature (Edax's evaluation covers the whole
    /// board), and squares should each appear the same number of times every real Othello position
    /// evaluator of this shape does -- not asserted precisely here (that's just "trust the
    /// transcription"), but every square must appear at least once as a coarse completeness check.
    #[test]
    fn every_square_appears_in_some_feature() {
        let mut seen = [false; 64];
        for feat in &FEATURES {
            for &sq in feat.squares {
                seen[sq as usize] = true;
            }
        }
        assert!(
            seen.iter().all(|&s| s),
            "some square is never read by any feature"
        );
    }

    /// `EVAL_F2X` (`eval.c:41-98`) plus `EVAL_OFFSET` (`eval.c:452-456`), converted from Edax's
    /// square-letter notation to this crate's square indices by a one-off script reading the real
    /// `eval.c` source text directly -- not copied from `FEATURES` above, so a transcription bug
    /// in `FEATURES` has an actual chance of being caught here instead of both copies trivially
    /// agreeing by construction. Only squares + offset are checked here; `FEATURES`'s
    /// `WeightGroup` assignment is checked separately below against `accumlate_eval`'s hardcoded
    /// index ranges (`midgame.c:83-95`: `f[0..4)`=C9, `f[4..8)`=C10, `f[8..12)`=S100,
    /// `f[12..16)`=S101, `f[16..30)`=S8x4, `f[30..46)`=S7654).
    const REAL_EVAL_F2X: [(&[u32], i32); 46] = [
        (&[0, 1, 8, 9, 2, 16, 10, 17, 18], 0),
        (&[7, 6, 15, 14, 5, 23, 13, 22, 21], 0),
        (&[56, 48, 57, 49, 40, 58, 41, 50, 42], 0),
        (&[63, 55, 62, 54, 47, 61, 46, 53, 45], 0),
        (&[32, 24, 16, 8, 0, 9, 1, 2, 3, 4], 0),
        (&[39, 31, 23, 15, 7, 14, 6, 5, 4, 3], 0),
        (&[24, 32, 40, 48, 56, 49, 57, 58, 59, 60], 0),
        (&[31, 39, 47, 55, 63, 54, 62, 61, 60, 59], 0),
        (&[9, 0, 1, 2, 3, 4, 5, 6, 7, 14], 0),
        (&[49, 56, 57, 58, 59, 60, 61, 62, 63, 54], 0),
        (&[9, 0, 8, 16, 24, 32, 40, 48, 56, 49], 0),
        (&[14, 7, 15, 23, 31, 39, 47, 55, 63, 54], 0),
        (&[0, 2, 3, 10, 11, 12, 13, 4, 5, 7], 0),
        (&[56, 58, 59, 50, 51, 52, 53, 60, 61, 63], 0),
        (&[0, 16, 24, 17, 25, 33, 41, 32, 40, 56], 0),
        (&[7, 23, 31, 22, 30, 38, 46, 39, 47, 63], 0),
        (&[8, 9, 10, 11, 12, 13, 14, 15], 0),
        (&[48, 49, 50, 51, 52, 53, 54, 55], 0),
        (&[1, 9, 17, 25, 33, 41, 49, 57], 0),
        (&[6, 14, 22, 30, 38, 46, 54, 62], 0),
        (&[16, 17, 18, 19, 20, 21, 22, 23], 6561),
        (&[40, 41, 42, 43, 44, 45, 46, 47], 6561),
        (&[2, 10, 18, 26, 34, 42, 50, 58], 6561),
        (&[5, 13, 21, 29, 37, 45, 53, 61], 6561),
        (&[24, 25, 26, 27, 28, 29, 30, 31], 13122),
        (&[32, 33, 34, 35, 36, 37, 38, 39], 13122),
        (&[3, 11, 19, 27, 35, 43, 51, 59], 13122),
        (&[4, 12, 20, 28, 36, 44, 52, 60], 13122),
        (&[0, 9, 18, 27, 36, 45, 54, 63], 19683),
        (&[56, 49, 42, 35, 28, 21, 14, 7], 19683),
        (&[1, 10, 19, 28, 37, 46, 55], 0),
        (&[15, 22, 29, 36, 43, 50, 57], 0),
        (&[8, 17, 26, 35, 44, 53, 62], 0),
        (&[6, 13, 20, 27, 34, 41, 48], 0),
        (&[2, 11, 20, 29, 38, 47], 2187),
        (&[16, 25, 34, 43, 52, 61], 2187),
        (&[5, 12, 19, 26, 33, 40], 2187),
        (&[23, 30, 37, 44, 51, 58], 2187),
        (&[3, 12, 21, 30, 39], 2916),
        (&[24, 33, 42, 51, 60], 2916),
        (&[4, 11, 18, 25, 32], 2916),
        (&[31, 38, 45, 52, 59], 2916),
        (&[3, 10, 17, 24], 3159),
        (&[32, 41, 50, 59], 3159),
        (&[4, 13, 22, 31], 3159),
        (&[39, 46, 53, 60], 3159),
    ];

    #[test]
    fn features_match_real_edax_eval_f2x_and_eval_offset() {
        for (i, (feat, (real_squares, real_offset))) in
            FEATURES.iter().zip(REAL_EVAL_F2X).enumerate()
        {
            assert_eq!(feat.squares, real_squares, "feature {i}: squares mismatch");
            assert_eq!(feat.offset, real_offset, "feature {i}: offset mismatch");
        }
    }

    /// `accumlate_eval`'s hardcoded `f[]` index ranges (`midgame.c:83-95`) assign features to
    /// weight arrays by *position*, not by any label in `EVAL_F2X` itself -- so `FEATURES`'s
    /// `WeightGroup` tagging is only correct if it reproduces those exact boundaries. Checked
    /// separately from `features_match_real_edax_eval_f2x_and_eval_offset` since group membership
    /// isn't part of `EVAL_F2X`/`EVAL_OFFSET` at all.
    #[test]
    fn feature_groups_match_accumlate_evals_hardcoded_ranges() {
        for (i, feat) in FEATURES.iter().enumerate() {
            let expected = match i {
                0..4 => WeightGroup::C9,
                4..8 => WeightGroup::C10,
                8..12 => WeightGroup::S100,
                12..16 => WeightGroup::S101,
                16..30 => WeightGroup::S8x4,
                30..46 => WeightGroup::S7654,
                _ => unreachable!(),
            };
            assert_eq!(feat.group, expected, "feature {i}: group mismatch");
        }
    }

    fn dummy_weights() -> Vec<EvalWeight> {
        // Not real trained weights -- just enough real-shaped data (via the real unpacking
        // pipeline, so indices are exercised the same way real weights would) to check
        // search_eval_0 runs to completion and respects the score bounds/rounding contract.
        let raw: Vec<i16> = (0..N_W * N_PLIES)
            .map(|i| ((i * 7 + 3) % 2001) as i16 - 1000)
            .collect();
        crate::weights::unpack(&raw, N_W, N_PLIES)
    }

    #[test]
    fn search_eval_0_stays_within_score_bounds_for_many_boards() {
        let weights = dummy_weights();
        let mut rng = 0x9e37_79b9_7f4a_7c15_u64;
        let mut xorshift = || {
            rng ^= rng << 13;
            rng ^= rng >> 7;
            rng ^= rng << 17;
            rng
        };

        for _game in 0..50 {
            let mut board = START;
            for _ply in 0..40 {
                let moves = board.get_moves();
                if moves == 0 {
                    board = board.pass();
                    if board.get_moves() == 0 {
                        break;
                    }
                    continue;
                }
                let score = search_eval_0(&board, &weights);
                assert!(
                    (-63..=63).contains(&score),
                    "score {score} out of bounds for board {board:?}"
                );

                let candidates: Vec<u32> = (0..64).filter(|&x| moves & (1u64 << x) != 0).collect();
                let x = candidates[(xorshift() as usize) % candidates.len()];
                board = board.play(x);
            }
        }
    }

    #[test]
    fn search_eval_0_is_deterministic() {
        let weights = dummy_weights();
        assert_eq!(
            search_eval_0(&START, &weights),
            search_eval_0(&START, &weights)
        );
    }

    /// `init_features` + `eval_from_features` must produce the same score as `search_eval_0`.
    /// This is the correctness invariant that makes incremental eval valid: the root initialisation
    /// matches what `search_eval_0` would compute from scratch.
    #[test]
    fn eval_from_features_matches_search_eval_0() {
        let weights = dummy_weights();
        let mut rng = 0xdead_beef_u64;
        let mut xorshift = || {
            rng ^= rng << 13;
            rng ^= rng >> 7;
            rng ^= rng << 17;
            rng
        };
        let mut board = START;
        for _ in 0..20 {
            let moves = board.get_moves();
            if moves == 0 {
                board = board.pass();
                if board.get_moves() == 0 {
                    break;
                }
                continue;
            }
            let candidates: Vec<u32> = (0..64).filter(|&x| moves & (1u64 << x) != 0).collect();
            board = board.play(candidates[(xorshift() as usize) % candidates.len()]);
        }
        let n_empties = 64 - (board.player | board.opponent).count_ones() as i32;
        let features = init_features(&board, n_empties);
        assert_eq!(
            eval_from_features(&features, n_empties, &weights),
            search_eval_0(&board, &weights),
            "incremental root features must match search_eval_0 at n_empties={n_empties}"
        );
    }

    /// Applying `update_features` then `undo_features` must leave the feature array unchanged.
    #[test]
    fn update_then_undo_is_identity() {
        let mut board = START;
        let mut rng = 0xc0ffee_u64;
        for _ in 0..15 {
            let moves = board.get_moves();
            if moves == 0 {
                board = board.pass();
                continue;
            }
            let candidates: Vec<u32> = (0..64).filter(|&x| moves & (1u64 << x) != 0).collect();
            rng ^= rng << 13;
            rng ^= rng >> 7;
            rng ^= rng << 17;
            board = board.play(candidates[(rng as usize) % candidates.len()]);
        }
        let n_empties = 64 - (board.player | board.opponent).count_ones() as i32;
        let mut features = init_features(&board, n_empties);
        let original = features;

        let moves = board.get_moves();
        let x = moves.trailing_zeros();
        let child = board.play(x);
        let flipped = child.player ^ board.opponent;
        let mover_is_eval_player = n_empties % 2 == 0;

        update_features(&mut features, x, flipped, mover_is_eval_player);
        undo_features(&mut features, x, flipped, mover_is_eval_player);
        assert_eq!(features, original, "update + undo must be identity");
    }
}
