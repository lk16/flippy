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

//! Alternate [`crate::board::Board::get_flip`] implementations, ported from Edax's other
//! `flip_*.c` files (`TASKS.md` Task 20), benchmarked against the dumb-fill baseline -- all three
//! measured slower, so none replace `Board::get_flip` (still the dumb-fill; see Task 20's
//! benchmark table). Kept in the tree, tested, and bit-exact-verified anyway: `Board::get_flip`'s
//! own doc comment already treats Edax's `flip_slow.c` this way (ported as `Board::get_flip`
//! itself, "exists specifically to assert equivalence in Edax's own test suite") -- these are the
//! same idea, three more independently-implemented oracles a future correctness bug in
//! `Board::get_flip` would have to fool all of simultaneously. [`get_flip_kindergarten`] and
//! [`get_flip_carry`] each shipped with a real bug during development, both caught by differential
//! testing before any benchmark ran (see their regression tests at the bottom of this file) -- the
//! exact kind of subtle, easy-to-miss error decision #2's "every mismatch is a bug" bar exists to
//! catch, so the tests that caught them stay even though the implementations they're testing lost
//! the benchmark.
//!
//! # Kindergarten (`flip_kindergarten`)
//!
//! Edax's actual default/portable fallback (`board.c`'s `#else // MOVE_GENERATOR_KINDERGARTEN`
//! branch) ships as 65 hand-unrolled per-square C functions with per-square-baked-in mask/shift
//! constants -- generated mechanically by `generate_flip.c` from one generic algorithm. Rather
//! than transcribing 65 near-duplicate functions (high hex-transcription risk for low benefit --
//! Rust/LLVM already specializes a generic function over compile-time-constant-like inputs just
//! as well as hand-duplicated C), this ports the *generator's own generic formulas*
//! (`generate_flip.c`'s `h_to_line`/`v_to_line`/`d7_to_line`/`d9_to_line`/`outflank`/`flip` plus
//! the per-square unrolling logic in its `main`), parameterized by the actual square at runtime
//! instead of unrolled at C-compile-time. The `OUTFLANK`/`FLIPPED` data tables below are copied
//! byte-for-byte from `flip_kindergarten.c` (mechanically extracted with a script, not hand-typed
//! -- and cross-checked identical against the same tables in `flip_bmi2.c`, a second independent
//! file in Edax's source).
//!
//! The algorithm, per line direction (horizontal/vertical/anti-diagonal `d7`/diagonal `d9`):
//! 1. Gather: compress the relevant line's opponent bits (middle 6 cells, excluding the line's own
//!    two endpoints) and player bits (all cells) into small integers, via a mask + multiply +
//!    shift (`h_to_line` needs no multiply -- a row is already a contiguous byte; `v_to_line` uses
//!    a column-position-dependent multiplier; `d7_to_line`/`d9_to_line` use a fixed multiplier,
//!    since diagonals are already "column-like" once masked).
//! 2. `OUTFLANK[line_pos][opponent_pattern] & player_pattern` -> `FLIPPED[line_pos][that]`: a
//!    table lookup identifying which of the line's cells flip (`line_pos` = the played square's
//!    0-7 position within that particular line).
//! 3. Scatter: place the resulting small bit pattern back onto the board at the right positions.
//!
//! # Carry/lzcnt hybrid (`flip_carry`)
//!
//! `flip_carry_64.c` and `flip_bitscan.c` both replace the table lookup with a tableless
//! arithmetic trick: propagate a carry through a run of contiguous opponent bits to find the
//! first non-opponent cell, then check whether it's a player disc. `flip_carry_64.c` uses this
//! only in the bit-index-increasing direction (handling the decreasing direction via `CONTIG_x`
//! tables instead); `flip_bitscan.c`'s `outflank_right` macro handles the decreasing direction
//! directly with a leading-zero-count. Since `wasm32` always has a fast native `i64.clz`
//! instruction (unlike x86 `lzcnt`, which needs a CPU feature check that `flip_bitscan.c` guards
//! for), this ports the lzcnt form for both files' shared "carry to find the first non-opponent
//! cell" idea rather than `flip_carry_64.c`'s CONTIG-table alternative -- one clean generic
//! algorithm capturing the technique both files use, not two near-duplicates of it.
//! (`flip_roxane.c` was examined too: same algorithm family again -- a per-direction tableless
//! add/last_bit trick -- but its masks are written in a square-numbering the file's own header
//! notes is "inverted compared to Edax's", and a worked example (A1's SE-direction mask) didn't
//! resolve cleanly against a from-scratch re-derivation in Edax's own numbering within a bounded
//! amount of re-checking. Given `flip_carry` already tests the "tableless carry trick" hypothesis
//! shared by that whole algorithm family, re-deriving Roxane's specific inverted-numbering variant
//! carried real transcription/off-by-one risk for no new algorithmic coverage, so it wasn't
//! separately ported.)

use crate::board::Board;
use std::sync::OnceLock;

/// `OUTFLANK[8][64]`, copied byte-for-byte from Edax's `flip_kindergarten.c` (mechanically
/// extracted from the real source file, not hand-typed; cross-checked identical against the same
/// table in `flip_bmi2.c`). `OUTFLANK[line_pos][opponent_6bit_pattern]` gives the candidate
/// bracketing player-disc position(s), to be ANDed against the actual player pattern.
#[rustfmt::skip]
const OUTFLANK: [[u8; 64]; 8] = [
    [0x00, 0x04, 0x00, 0x08, 0x00, 0x04, 0x00, 0x10, 0x00, 0x04, 0x00, 0x08, 0x00, 0x04, 0x00, 0x20, 0x00, 0x04, 0x00, 0x08, 0x00, 0x04, 0x00, 0x10, 0x00, 0x04, 0x00, 0x08, 0x00, 0x04, 0x00, 0x40, 0x00, 0x04, 0x00, 0x08, 0x00, 0x04, 0x00, 0x10, 0x00, 0x04, 0x00, 0x08, 0x00, 0x04, 0x00, 0x20, 0x00, 0x04, 0x00, 0x08, 0x00, 0x04, 0x00, 0x10, 0x00, 0x04, 0x00, 0x08, 0x00, 0x04, 0x00, 0x80],
    [0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x80, 0x00],
    [0x00, 0x01, 0x00, 0x00, 0x10, 0x11, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x20, 0x21, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x10, 0x11, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x40, 0x41, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x10, 0x11, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x20, 0x21, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x10, 0x11, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x80, 0x81, 0x00, 0x00],
    [0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x20, 0x20, 0x22, 0x21, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x42, 0x41, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x20, 0x20, 0x22, 0x21, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x80, 0x80, 0x82, 0x81, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x40, 0x40, 0x44, 0x44, 0x42, 0x41, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x80, 0x80, 0x80, 0x84, 0x84, 0x82, 0x81, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x08, 0x08, 0x08, 0x04, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x88, 0x88, 0x88, 0x88, 0x84, 0x84, 0x82, 0x81, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x08, 0x08, 0x08, 0x08, 0x04, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x08, 0x08, 0x08, 0x08, 0x04, 0x04, 0x02, 0x01],
];

/// See [`OUTFLANK`]'s doc comment.
#[rustfmt::skip]
const FLIPPED: [[u8; 144]; 8] = [
    [0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0e, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1e, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3e, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0c, 0x0d, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1c, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3c, 0x3d, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x03, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x0b, 0x0a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x1b, 0x1a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x38, 0x3b, 0x3a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x07, 0x06, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x17, 0x16, 0x00, 0x14, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x30, 0x37, 0x36, 0x00, 0x34, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x0f, 0x0e, 0x00, 0x0c, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x2f, 0x2e, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x1f, 0x1e, 0x00, 0x1c, 0x00, 0x00, 0x00, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
    [0x00, 0x3f, 0x3e, 0x00, 0x3c, 0x00, 0x00, 0x00, 0x38, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
];

/// Absolute-row/col mask excluding row 0, row 7, col A, col H -- masking a *diagonal*'s full mask
/// to this always removes exactly that diagonal's own two endpoints, regardless of the
/// diagonal's length (a diagonal's endpoint always sits on some board edge -- row in {0,7} or col
/// in {A,H} -- so it's always excluded by at least one of the four exclusions). Matches
/// `generate_flip.c`'s `d7_mask(x,1,6)`/`d9_mask(x,1,6)` calls (their `(1,6)` bounds are absolute
/// row/col bounds, not line-relative ones -- this constant *is* that restriction). Diagonal-only:
/// a *column* has constant column, so excluding col A/H would zero out the whole column whenever
/// that column itself is A or H, rather than just its endpoints -- see [`ROW_MID_MASK`] instead
/// (this exact mistake was `get_flip_kindergarten`'s second bug during development; see this
/// module's regression tests).
const MID_MASK: u64 = 0x007e_7e7e_7e7e_7e00;

/// Row-only counterpart of [`MID_MASK`], for the vertical line: excludes row 0 and row 7 (a
/// column's own two endpoints), leaving all 8 columns intact.
const ROW_MID_MASK: u64 = 0x00ff_ffff_ffff_ff00;

/// Full diagonal masks through every square, for both diagonal directions (`d9` = the direction
/// [`Board::get_flip`] calls SE/NW, step 9; `d7` = SW/NE, step 7). Computed once (`generate_flip.c`
/// builds the equivalent via its own `d7_mask`/`d9_mask` helpers; this is the same geometric mask,
/// derived independently by extending a ray both ways from each square to the board edge).
fn diag_masks() -> &'static ([u64; 64], [u64; 64]) {
    static MASKS: OnceLock<([u64; 64], [u64; 64])> = OnceLock::new();
    MASKS.get_or_init(|| {
        let mut d9 = [0u64; 64];
        let mut d7 = [0u64; 64];
        for x in 0..64i32 {
            let row = x / 8;
            let col = x % 8;

            let mut m9 = 0u64;
            let (mut r, mut c) = (row, col);
            while r >= 0 && c >= 0 {
                m9 |= 1u64 << (r * 8 + c);
                r -= 1;
                c -= 1;
            }
            let (mut r, mut c) = (row + 1, col + 1);
            while r < 8 && c < 8 {
                m9 |= 1u64 << (r * 8 + c);
                r += 1;
                c += 1;
            }
            d9[x as usize] = m9;

            let mut m7 = 0u64;
            let (mut r, mut c) = (row, col);
            while r < 8 && c >= 0 {
                m7 |= 1u64 << (r * 8 + c);
                r += 1;
                c -= 1;
            }
            let (mut r, mut c) = (row - 1, col + 1);
            while r >= 0 && c < 8 {
                m7 |= 1u64 << (r * 8 + c);
                r -= 1;
                c += 1;
            }
            d7[x as usize] = m7;
        }
        (d9, d7)
    })
}

/// Scatter a `FLIPPED[..]` byte (bit `k` means absolute line-position `k+1`, 0-indexed, per
/// [`OUTFLANK`]/[`FLIPPED`]'s doc comment) onto the board, for a line whose position-0 cell sits
/// at board bit `start_bit` and whose consecutive line positions are `step` board-bits apart.
/// Used for the vertical line (`step=8`, `start_bit` = the square's own column, row 0) and both
/// diagonals (`step=9`/`7`, `start_bit` = that diagonal's topmost cell). Horizontal doesn't need
/// this: a row is already a contiguous byte, so [`Board::get_flip`]'s kindergarten port scatters
/// it with one plain shift instead.
fn scatter(v: u8, start_bit: u32, step: u32) -> u64 {
    let mut r = 0u64;
    for k in 0..7u32 {
        if v & (1 << k) != 0 {
            r |= 1u64 << (start_bit + step * (k + 1));
        }
    }
    r
}

/// Gather a line's cells into [`OUTFLANK`]'s query convention: bit `k` means "line-position
/// `k+1` is set" (`k` = 0..=5, covering the line's middle positions 1..6 -- positions 0 and 7,
/// the line's own two endpoints, are never part of an `OUTFLANK` query). `masked` must already be
/// ANDed with that line's [`MID_MASK`]-restricted mask.
///
/// This is a diagonal-only helper: horizontal doesn't need it (a row is already a contiguous
/// byte, shifted directly), and vertical's fixed byte-per-row stride makes the "gather via
/// multiply" trick unambiguous (verified empirically: for `step=8`, the multiply always produces
/// this exact bit ordering, for every column). Diagonals (`step=9`/`7`) don't share that
/// property: the multiply trick's output bit order depends on the diagonal's own start-bit
/// alignment mod 8 (confirmed by hand for two different diagonals -- one gave a straightforward
/// "+1 offset" ordering, another gave a fully bit-reversed one), which is exactly why Edax's own
/// generated code uses a different precomputed unpack array per diagonal group instead of one
/// formula. Rather than re-deriving the right per-alignment correction, this gathers by directly
/// testing each line-relative position -- unambiguous by construction, at the cost of a small
/// fixed loop instead of one multiply. (This was `get_flip_kindergarten`'s first bug during
/// development: an earlier version reused the vertical multiply trick for diagonals too, and
/// failed differential testing -- see this module's regression tests.)
fn gather_mid(masked: u64, start_bit: u32, step: u32) -> u8 {
    let mut v = 0u8;
    for k in 0..6u32 {
        let pos = start_bit + step * (k + 1);
        if pos < 64 && masked & (1u64 << pos) != 0 {
            v |= 1 << k;
        }
    }
    v
}

/// Gather a line's cells into `OUTFLANK`'s raw-output convention: bit `y` means "line-position
/// `y` is set" directly, unshifted (`y` = 0..=7, covering all 8 positions including both
/// endpoints -- the bracketing player disc can be at either end of the line). `masked` must
/// already be ANDed with that line's full mask. See [`gather_mid`]'s doc comment for why this is
/// a loop rather than the multiply trick [`get_flip_kindergarten`] uses for horizontal/vertical.
fn gather_full(masked: u64, start_bit: u32, step: u32) -> u8 {
    let mut v = 0u8;
    for y in 0..8u32 {
        let pos = start_bit + step * y;
        if pos < 64 && masked & (1u64 << pos) != 0 {
            v |= 1 << y;
        }
    }
    v
}

/// Port of Edax's default/portable `flip_kindergarten.c` (see this module's doc comment) --
/// generic over the played square instead of unrolled into 65 per-square functions.
pub fn get_flip_kindergarten(board: &Board, x: u32) -> u64 {
    let p = board.player;
    let o = board.opponent;
    let row = x / 8;
    let col = x % 8;
    let row_shift = row * 8;

    // Horizontal: a row is already a contiguous byte, no gather multiply needed.
    let idx_h = (OUTFLANK[col as usize][((o >> (row_shift + 1)) & 0x3f) as usize]
        & ((p >> row_shift) as u8)) as usize;
    let mut flipped = (FLIPPED[col as usize][idx_h] as u64) << (row_shift + 1);

    // Vertical: gather via a column-position-dependent multiplier (`generate_flip.c`'s
    // `v_to_line`), scatter via the generic `scatter` helper (start_bit = column, row 0; step 8).
    let full_v = 0x0101_0101_0101_0101u64 << col;
    let mid_v = full_v & ROW_MID_MASK;
    let mult_p = 0x0102_0408_1020_4080u64 >> col;
    let mult_o = 0x0002_0408_1020_4000u64 >> col;
    let idx_v = (OUTFLANK[row as usize][((o & mid_v).wrapping_mul(mult_o) >> 57) as usize]
        & ((p & full_v).wrapping_mul(mult_p) >> 56) as u8) as usize;
    flipped |= scatter(FLIPPED[row as usize][idx_v], col, 8);

    // Diagonals: gather via the explicit per-position loop (see `gather_mid`/`gather_full`'s doc
    // comment for why this can't reuse vertical's multiply trick).
    let (d9_full, d7_full) = diag_masks();

    let full_9 = d9_full[x as usize];
    let mid_9 = full_9 & MID_MASK;
    let start_9 = full_9.trailing_zeros();
    let pos_9 = row.min(col);
    let idx_9 = (OUTFLANK[pos_9 as usize][gather_mid(o & mid_9, start_9, 9) as usize]
        & gather_full(p & full_9, start_9, 9)) as usize;
    flipped |= scatter(FLIPPED[pos_9 as usize][idx_9], start_9, 9);

    let full_7 = d7_full[x as usize];
    let mid_7 = full_7 & MID_MASK;
    let start_7 = full_7.trailing_zeros();
    let pos_7 = row.min(7 - col);
    let idx_7 = (OUTFLANK[pos_7 as usize][gather_mid(o & mid_7, start_7, 7) as usize]
        & gather_full(p & full_7, start_7, 7)) as usize;
    flipped |= scatter(FLIPPED[pos_7 as usize][idx_7], start_7, 7);

    flipped
}

/// Full-ray masks (all 8 directions) through every square, each extended from just beyond the
/// square to the board edge -- e.g. direction index 0 (East) from square `x` is every square
/// strictly to the right of `x` on its row. Used by [`get_flip_carry`]'s carry/lzcnt trick, which
/// needs to know exactly where each ray ends (unlike the dumb-fill's shift-based masks, this is
/// computed from board coordinates directly, so it can't wrap across an edge by construction).
fn ray_masks() -> &'static [[u64; 8]; 64] {
    static MASKS: OnceLock<[[u64; 8]; 64]> = OnceLock::new();
    MASKS.get_or_init(|| {
        // Index order: 0=E, 1=W, 2=S, 3=N, 4=SE, 5=NW, 6=SW, 7=NE.
        const STEPS: [(i32, i32); 8] = [
            (0, 1),
            (0, -1),
            (1, 0),
            (-1, 0),
            (1, 1),
            (-1, -1),
            (1, -1),
            (-1, 1),
        ];
        let mut result = [[0u64; 8]; 64];
        for (x, dirs) in result.iter_mut().enumerate() {
            let row0 = (x / 8) as i32;
            let col0 = (x % 8) as i32;
            for (dst, &(dr, dc)) in dirs.iter_mut().zip(STEPS.iter()) {
                let mut m = 0u64;
                let (mut r, mut c) = (row0 + dr, col0 + dc);
                while (0..8).contains(&r) && (0..8).contains(&c) {
                    m |= 1u64 << (r * 8 + c);
                    r += dr;
                    c += dc;
                }
                *dst = m;
            }
        }
        result
    })
}

/// Port of the "carry to find the first non-opponent cell" technique shared by
/// `flip_carry_64.c` and `flip_bitscan.c` (see this module's doc comment).
pub fn get_flip_carry(board: &Board, x: u32) -> u64 {
    let p = board.player;
    let o = board.opponent;
    let masks = &ray_masks()[x as usize];
    let mut flipped = 0u64;

    // Forward directions (E, S, SE, SW -- bit index increases): propagate a carry through
    // contiguous opponent bits via `+1`; it lands on the first non-opponent cell within `mask`
    // (or overflows past bit 63 to 0, if the whole ray to the edge is opponent-occupied).
    for &mask in &[masks[0], masks[2], masks[4], masks[6]] {
        let outflank = (o | !mask).wrapping_add(1) & p & mask;
        if outflank != 0 {
            flipped |= outflank.wrapping_sub(1) & mask;
        }
    }

    // Backward directions (W, N, NW, NE -- bit index decreases): the mirror trick, using a
    // leading-zero-count (wasm has a fast native `i64.clz`, so this needs no CPU feature check,
    // unlike x86 `lzcnt`) to find the first non-opponent cell counting down from `x`.
    for &mask in &[masks[1], masks[3], masks[5], masks[7]] {
        let non_opponent = !o & mask;
        if non_opponent == 0 {
            continue; // whole ray to the edge is opponent-occupied (or there's no ray at all)
        }
        let candidate = 0x8000_0000_0000_0000u64 >> non_opponent.leading_zeros();
        let outflank = candidate & p & mask;
        if outflank != 0 {
            flipped |= mask & !(outflank | outflank.wrapping_sub(1));
        }
    }

    flipped
}

/// Kogge-Stone doubling occluded fill -- not an Edax port (see this module's doc comment header
/// for why it's included alongside the two that are): a generic bitboard sliding-piece technique
/// from chess programming, benchmarked here for a fair three-way comparison against
/// [`get_flip_kindergarten`] and [`get_flip_carry`] in the same session.
pub fn get_flip_kogge_stone(board: &Board, x: u32) -> u64 {
    const NOT_AH_FILE: u64 = 0x7e7e_7e7e_7e7e_7e7e;
    let bit = 1u64 << x;
    let p = board.player;
    let om = board.opponent & NOT_AH_FILE;
    let ov = board.opponent;

    let mut flipped = 0u64;

    let mut gen = bit;
    let mut pro = om;
    let step = (gen << 1) & pro;
    if step != 0 {
        gen |= step;
        pro &= pro << 1;
        gen |= pro & (gen << 2);
        pro &= pro << 2;
        gen |= pro & (gen << 4);
        if (gen << 1) & p != 0 {
            flipped |= gen ^ bit;
        }
    }

    let mut gen = bit;
    let mut pro = om;
    let step = (gen >> 1) & pro;
    if step != 0 {
        gen |= step;
        pro &= pro >> 1;
        gen |= pro & (gen >> 2);
        pro &= pro >> 2;
        gen |= pro & (gen >> 4);
        if (gen >> 1) & p != 0 {
            flipped |= gen ^ bit;
        }
    }

    let mut gen = bit;
    let mut pro = ov;
    let step = (gen << 8) & pro;
    if step != 0 {
        gen |= step;
        pro &= pro << 8;
        gen |= pro & (gen << 16);
        pro &= pro << 16;
        gen |= pro & (gen << 32);
        if (gen << 8) & p != 0 {
            flipped |= gen ^ bit;
        }
    }

    let mut gen = bit;
    let mut pro = ov;
    let step = (gen >> 8) & pro;
    if step != 0 {
        gen |= step;
        pro &= pro >> 8;
        gen |= pro & (gen >> 16);
        pro &= pro >> 16;
        gen |= pro & (gen >> 32);
        if (gen >> 8) & p != 0 {
            flipped |= gen ^ bit;
        }
    }

    let mut gen = bit;
    let mut pro = om;
    let step = (gen << 9) & pro;
    if step != 0 {
        gen |= step;
        pro &= pro << 9;
        gen |= pro & (gen << 18);
        pro &= pro << 18;
        gen |= pro & (gen << 36);
        if (gen << 9) & p != 0 {
            flipped |= gen ^ bit;
        }
    }

    let mut gen = bit;
    let mut pro = om;
    let step = (gen >> 9) & pro;
    if step != 0 {
        gen |= step;
        pro &= pro >> 9;
        gen |= pro & (gen >> 18);
        pro &= pro >> 18;
        gen |= pro & (gen >> 36);
        if (gen >> 9) & p != 0 {
            flipped |= gen ^ bit;
        }
    }

    let mut gen = bit;
    let mut pro = om;
    let step = (gen << 7) & pro;
    if step != 0 {
        gen |= step;
        pro &= pro << 7;
        gen |= pro & (gen << 14);
        pro &= pro << 14;
        gen |= pro & (gen << 28);
        if (gen << 7) & p != 0 {
            flipped |= gen ^ bit;
        }
    }

    let mut gen = bit;
    let mut pro = om;
    let step = (gen >> 7) & pro;
    if step != 0 {
        gen |= step;
        pro &= pro >> 7;
        gen |= pro & (gen >> 14);
        pro &= pro >> 14;
        gen |= pro & (gen >> 28);
        if (gen >> 7) & p != 0 {
            flipped |= gen ^ bit;
        }
    }

    flipped
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::board::START;

    /// Plays a deterministic pseudo-random sequence of legal moves (mirroring `board.rs`'s own
    /// `matches_independent_reference_across_random_self_play_games` test) and, at every ply,
    /// checks every candidate square's flip result against [`Board::get_flip`] (already verified
    /// bit-exact against real Edax by the differential harness) -- the correctness bar for an
    /// alternate implementation that's never wired into `Board` itself.
    fn check_against_reference(get_flip: impl Fn(&Board, u32) -> u64) {
        fn next(state: &mut u64) -> u64 {
            *state ^= *state << 13;
            *state ^= *state >> 7;
            *state ^= *state << 17;
            *state
        }

        for game in 0..200u64 {
            let mut board = START;
            let mut state = 88172645463325252u64.wrapping_add(game);
            for _ply in 0..100 {
                let moves = board.get_moves();
                if moves == 0 {
                    let passed = board.pass();
                    if passed.get_moves() == 0 {
                        break;
                    }
                    board = passed;
                    continue;
                }
                for sq in 0..64u32 {
                    if moves & (1 << sq) != 0 {
                        assert_eq!(
                            get_flip(&board, sq),
                            board.get_flip(sq),
                            "mismatch at square {sq} on board {board:?}"
                        );
                    }
                }
                let candidates: Vec<u32> = (0..64).filter(|&sq| moves & (1 << sq) != 0).collect();
                let choice = candidates[(next(&mut state) as usize) % candidates.len()];
                board = board.play(choice);
            }
        }
    }

    #[test]
    fn kindergarten_matches_reference_get_flip() {
        check_against_reference(get_flip_kindergarten);
    }

    #[test]
    fn carry_matches_reference_get_flip() {
        check_against_reference(get_flip_carry);
    }

    #[test]
    fn kogge_stone_matches_reference_get_flip() {
        check_against_reference(get_flip_kogge_stone);
    }

    /// Regression test for `get_flip_kindergarten`'s first development bug (Task 20's second
    /// investigation, 2026-08-04): an earlier version reused the vertical line's "gather via
    /// multiply" trick for diagonals too. That trick only produces the assumed "bit `k` = line
    /// position `k+1`" ordering when the line's start bit is byte-aligned (true for every column,
    /// since a column always starts at row 0); diagonals step by 9 or 7, not 8, so the same
    /// multiply produces a *rotated or fully bit-reversed* bit order depending on the specific
    /// diagonal's start-bit alignment mod 8. This board+square is the exact case that first
    /// exposed it: playing at F2 (square 13) should flip E3 via the `d7` diagonal (F2-E3-D4, with
    /// D4 the bracketing player disc); the buggy multiply-based gather produced no flip at all.
    #[test]
    fn kindergarten_diagonal_bit_ordering_regression() {
        let board = Board {
            player: 34_494_480_384,
            opponent: 68_988_960_768,
        };
        let x = 13; // F2
        let expected = 1u64 << 20; // E3

        assert_eq!(
            board.get_flip(x),
            expected,
            "reference get_flip disagrees with the fixture"
        );
        assert_eq!(get_flip_kindergarten(&board, x), expected);
    }

    /// Regression test for `get_flip_kindergarten`'s second development bug (same investigation):
    /// the vertical line reused [`MID_MASK`] (built for diagonals, where excluding column A and H
    /// is correct because a diagonal's two endpoints can sit on any of the four board edges).
    /// Applied to a *column*, whose column is constant, excluding column A or H zeroes the entire
    /// gather whenever the played square is itself in column A or H -- not just that column's own
    /// endpoints. This board+square is the exact case that first exposed it: playing at A6
    /// (square 40, column A) should flip B6 and C6 horizontally and A7 vertically (A7 opponent,
    /// A8 the bracketing player disc); with the bug, the vertical contribution (A7) was silently
    /// dropped because `mid_v` came out zero for column A.
    #[test]
    fn kindergarten_vertical_edge_column_regression() {
        let board = Board {
            player: 4_553_165_715_441_500_290,
            opponent: 9_245_298_939_280_634_884,
        };
        let x = 40; // A6
        let expected = (1u64 << 41) | (1u64 << 42) | (1u64 << 48); // B6, C6, A7

        assert_eq!(
            board.get_flip(x),
            expected,
            "reference get_flip disagrees with the fixture"
        );
        assert_eq!(get_flip_kindergarten(&board, x), expected);
    }
}
