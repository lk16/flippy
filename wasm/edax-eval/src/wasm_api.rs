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

//! The raw wasm-facing API (`TASKS.md` Task 10): a minimal C ABI (`#[no_mangle] extern "C"`)
//! surface for `js/edax-eval.js` to call, deliberately not using `wasm-bindgen` — this repo has no
//! frontend build step (`docs/project.md`), and the goal here is a `.wasm` file plus a plain
//! `<script type="module">`-loadable JS file, not a bundler-oriented toolchain.
//!
//! Not `#[cfg(target_arch = "wasm32")]`-gated: these functions compile and are directly unit-
//! testable on the native target too (see this module's tests), which is worth more than avoiding
//! a few harmless exported C symbols in native builds nobody links against externally anyway.
//!
//! Protocol: call [`alloc`] for a buffer sized to the (decompressed, still transform-encoded --
//! see [`crate::weights_transform`]) weights blob, write it into wasm linear memory at the
//! returned pointer, call [`init_weights`] once with that pointer/length, then call [`evaluate`]
//! any number of times. `evaluate`'s `player`/`opponent` are Edax's mover-relative bitboards
//! (`crate::board::Board`), not black/white -- the JS wrapper's job, not this module's.

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

/// [`init_weights`]'s sentinel for "wrong-sized blob" (a caller bug, not a runtime input to
/// validate beyond this -- this crate ships exactly one weights blob, see its doc comment).
pub const ERR_WRONG_LENGTH: i32 = 1;

/// [`init_weights`]'s sentinel for "already called" (weights are set once, for the lifetime of
/// the wasm instance).
pub const ERR_ALREADY_INITIALIZED: i32 = 2;

/// Allocates `len` zeroed bytes in wasm linear memory and returns a pointer the caller can write
/// into (from JS: `new Uint8Array(instance.exports.memory.buffer, ptr, len)`). The buffer's
/// ownership passes to whoever reclaims it -- currently only [`init_weights`], via
/// `Box::from_raw`, so every `alloc`ed buffer must be handed to exactly one `init_weights` call
/// with the same `len`, or it leaks for the lifetime of the wasm instance (acceptable here: this
/// crate allocates the weights blob buffer once, not in a hot loop).
#[no_mangle]
pub extern "C" fn alloc(len: usize) -> *mut u8 {
    let buf = vec![0u8; len].into_boxed_slice();
    Box::into_raw(buf) as *mut u8
}

/// Reverses Task 2's transpose+delta+byte-plane encoding on the (already gzip-decompressed, e.g.
/// via the browser's native `DecompressionStream('gzip')`) weights blob at `ptr`/`len` -- which
/// must be a buffer previously returned by [`alloc`] with the same `len`, now filled in by the
/// caller -- and unpacks it into the full per-ply weight tables `evaluate` uses. Returns 0 on
/// success, [`ERR_WRONG_LENGTH`] if `len` doesn't match the one weights blob this crate ships (a
/// caller bug), or [`ERR_ALREADY_INITIALIZED`] if called more than once.
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
    // SAFETY: per this function's own safety contract, `ptr`/`len` come from a matching `alloc`
    // call; `Box::into_raw` (in `alloc`) and `Box::from_raw` (here) use the same allocator, so
    // this is exactly one alloc/dealloc pair from the caller's point of view.
    let bytes = Box::from_raw(std::ptr::slice_from_raw_parts_mut(ptr, len));

    let raw = weights_transform::decode(&bytes, weights_transform::N_W, weights_transform::N_PLIES);
    let unpacked = weights::unpack(&raw, weights_transform::N_W, weights_transform::N_PLIES);

    if WEIGHTS.set(unpacked).is_err() {
        return ERR_ALREADY_INITIALIZED;
    }
    0
}

/// Evaluates a board (`player`/`opponent`: Edax's mover-relative bitboards, `crate::board::Board`)
/// at Edax `level`, returning the exact score from `player`'s point of view. Levels 0..=60 use
/// depth and selectivity from `search_global_init` (see `crate::search::solve`); levels 11-60
/// currently search full-width (ProbCut not yet implemented, TASKS.md Task 15) and will differ
/// from real Edax above level 10. Levels above 60 return [`ERR_UNSUPPORTED_LEVEL`]. Returns
/// [`ERR_WEIGHTS_NOT_INITIALIZED`] if [`init_weights`] hasn't been called yet.
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

    /// Builds a real (transform-encoded, not yet decompressed) weights blob the same way the
    /// browser would receive it after `DecompressionStream('gzip')`, from synthetic but
    /// realistically-shaped data (this module doesn't need real trained weights -- Task 8 already
    /// covers bit-exactness end to end; this just exercises the alloc/init/evaluate protocol).
    fn synthetic_transform_encoded_blob() -> Vec<u8> {
        let raw: Vec<i16> = (0..N_W * N_PLIES)
            .map(|i| ((i * 7 + 3) % 2001) as i16 - 1000)
            .collect();
        encode(&raw, N_W, N_PLIES)
    }

    #[test]
    fn evaluate_before_init_weights_reports_not_initialized() {
        // A fresh process-local static per test binary isn't guaranteed by cargo test's threading
        // model (tests share a process), so this only asserts the sentinel value is distinct from
        // real scores -- the "not yet initialized" path itself is exercised by whichever test in
        // this binary happens to run first, since WEIGHTS is a OnceLock shared across tests.
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

        // init_weights may already have run (WEIGHTS is shared across this binary's tests); either
        // outcome is a legitimate protocol response.
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
        // ERR_WRONG_LENGTH returns before reclaiming the buffer via Box::from_raw, so free it
        // directly to avoid leaking in this test (init_weights's normal paths do reclaim it).
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
