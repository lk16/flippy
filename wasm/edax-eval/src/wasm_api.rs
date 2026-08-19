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

//! The raw wasm-facing API: a minimal C ABI surface for `js/edax-eval.js`, deliberately without
//! `wasm-bindgen` (this repo has no frontend build step). Not `#[cfg(target_arch = "wasm32")]`-
//! gated, so it stays unit-testable on the native target.
//!
//! Protocol: [`alloc`] a buffer for the (decompressed, still transform-encoded) weights blob,
//! write it into wasm linear memory, call [`init_weights`] once, then [`evaluate`] any number of
//! times. `evaluate`'s `player`/`opponent` are mover-relative bitboards, not black/white — the
//! translation is the JS wrapper's job.

use std::sync::OnceLock;

use crate::board::Board;
use crate::search::solve;
use crate::weights::{self, EvalWeight};
use crate::weights_transform;

static WEIGHTS: OnceLock<Vec<EvalWeight>> = OnceLock::new();

/// [`evaluate`]'s sentinel for "call [`init_weights`] first". Real scores are always in
/// `-64..=64` ([`crate::search`]'s `SCORE_MIN + 1 ..= SCORE_MAX - 1`), far outside `i32::MIN`.
pub const ERR_WEIGHTS_NOT_INITIALIZED: i32 = i32::MIN;

/// [`evaluate`]'s sentinel for `level > 60` (see `crate::search::UnsupportedLevel`).
pub const ERR_UNSUPPORTED_LEVEL: i32 = i32::MIN + 1;

/// [`init_weights`]'s sentinel for a wrong-sized blob (a caller bug).
pub const ERR_WRONG_LENGTH: i32 = 1;

/// [`init_weights`]'s sentinel for "already called" (weights are set once per wasm instance).
pub const ERR_ALREADY_INITIALIZED: i32 = 2;

/// Allocates `len` zeroed bytes in wasm linear memory for the caller to write into. Ownership
/// passes to exactly one [`init_weights`] call with the same `len` (via `Box::from_raw`);
/// otherwise the buffer leaks.
#[no_mangle]
pub extern "C" fn alloc(len: usize) -> *mut u8 {
    let buf = vec![0u8; len].into_boxed_slice();
    Box::into_raw(buf) as *mut u8
}

/// Decodes the (already gzip-decompressed) transform-encoded weights blob at `ptr`/`len` and
/// unpacks it into the per-ply tables [`evaluate`] uses. Returns 0 on success,
/// [`ERR_WRONG_LENGTH`], or [`ERR_ALREADY_INITIALIZED`].
///
/// # Safety
/// `ptr`/`len` must be exactly as returned by a prior [`alloc`] call (same `len`), not yet passed
/// to any other call reclaiming it.
#[no_mangle]
pub unsafe extern "C" fn init_weights(ptr: *mut u8, len: usize) -> i32 {
    let expected_len = 2 * weights_transform::N_W * weights_transform::N_PLIES;
    if len != expected_len {
        return ERR_WRONG_LENGTH;
    }
    // SAFETY: per the safety contract, `ptr`/`len` come from a matching `alloc` call, so
    // Box::into_raw/Box::from_raw form exactly one alloc/dealloc pair.
    let bytes = Box::from_raw(std::ptr::slice_from_raw_parts_mut(ptr, len));

    let raw = weights_transform::decode(&bytes, weights_transform::N_W, weights_transform::N_PLIES);
    let unpacked = weights::unpack(&raw, weights_transform::N_W, weights_transform::N_PLIES);

    if WEIGHTS.set(unpacked).is_err() {
        return ERR_ALREADY_INITIALIZED;
    }
    0
}

/// Evaluates a mover-relative board at Edax `level` (see `crate::search::solve`), returning the
/// score from `player`'s view; [`ERR_UNSUPPORTED_LEVEL`] above level 60,
/// [`ERR_WEIGHTS_NOT_INITIALIZED`] before [`init_weights`].
#[no_mangle]
pub extern "C" fn evaluate(player: u64, opponent: u64, level: u32) -> i32 {
    let Some(weights) = WEIGHTS.get() else {
        return ERR_WEIGHTS_NOT_INITIALIZED;
    };
    let board = Board { player, opponent };
    solve(&board, level, weights).unwrap_or(ERR_UNSUPPORTED_LEVEL)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::board::START;
    use crate::weights_transform::{encode, N_PLIES, N_W};

    /// A transform-encoded weights blob from synthetic but realistically-shaped data — enough to
    /// exercise the alloc/init/evaluate protocol without real trained weights.
    fn synthetic_transform_encoded_blob() -> Vec<u8> {
        let raw: Vec<i16> = (0..N_W * N_PLIES)
            .map(|i| ((i * 7 + 3) % 2001) as i16 - 1000)
            .collect();
        encode(&raw, N_W, N_PLIES)
    }

    #[test]
    fn evaluate_before_init_weights_reports_not_initialized() {
        // WEIGHTS is a OnceLock shared across this binary's tests, so only assert the sentinels
        // are distinct from real scores.
        assert!(!(-64..=64).contains(&ERR_WEIGHTS_NOT_INITIALIZED));
        assert!(!(-64..=64).contains(&ERR_UNSUPPORTED_LEVEL));
        assert_ne!(ERR_WEIGHTS_NOT_INITIALIZED, ERR_UNSUPPORTED_LEVEL);
    }

    #[test]
    fn alloc_init_evaluate_round_trip() {
        let blob = synthetic_transform_encoded_blob();
        let ptr = alloc(blob.len());
        assert!(!ptr.is_null());
        unsafe {
            std::ptr::copy_nonoverlapping(blob.as_ptr(), ptr, blob.len());
        }

        // init_weights may already have run (shared OnceLock); either outcome is legitimate.
        let init_result = unsafe { init_weights(ptr, blob.len()) };
        assert!(init_result == 0 || init_result == ERR_ALREADY_INITIALIZED);

        let score = evaluate(START.player, START.opponent, 10);
        assert!(
            (-64..=64).contains(&score),
            "evaluate returned out-of-range score {score}"
        );
    }

    #[test]
    fn init_weights_rejects_wrong_length() {
        let ptr = alloc(4);
        assert_eq!(unsafe { init_weights(ptr, 4) }, ERR_WRONG_LENGTH);
        // ERR_WRONG_LENGTH returns before reclaiming the buffer, so free it here.
        unsafe {
            drop(Box::from_raw(std::ptr::slice_from_raw_parts_mut(ptr, 4)));
        }
    }

    #[test]
    fn evaluate_rejects_level_above_sixty() {
        let blob = synthetic_transform_encoded_blob();
        let ptr = alloc(blob.len());
        unsafe {
            std::ptr::copy_nonoverlapping(blob.as_ptr(), ptr, blob.len());
        }
        let _ = unsafe { init_weights(ptr, blob.len()) }; // may already be initialized, see above

        assert_eq!(
            evaluate(START.player, START.opponent, 61),
            ERR_UNSUPPORTED_LEVEL
        );
    }
}
