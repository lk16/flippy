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

//! Reversible transform applied to the raw, ply-major slice of `eval.dat`'s
//! packed weights before compression (`TASKS.md` Task 2). The slice is
//! transposed to weight-major order, delta-encoded across the ply axis, and
//! byte-plane split; see `TASKS.md`'s "Weights file" section for the
//! rationale and measured sizes against the real file.
//!
//! Used by `src/bin/extract_weights.rs` (encode, offline) and, from Task 4
//! onward, by the wasm module itself (decode, in-browser).

/// Number of packed weights per ply (`eval.c`'s `n_w`).
pub const N_W: usize = 114364;

/// Number of plies `eval_open` actually uses: ply 2..53 inclusive (`EVAL_N_PLY - 2`).
pub const N_PLIES: usize = 52;

/// Transposes `raw` (ply-major: `raw[ply * n_w + weight]`) to weight-major order, delta-encodes
/// each weight's `n_plies`-long series across the ply axis, and splits the resulting `i16`s into
/// a low-byte plane followed by a high-byte plane.
///
/// Panics if `raw.len() != n_w * n_plies`.
pub fn encode(raw: &[i16], n_w: usize, n_plies: usize) -> Vec<u8> {
    assert_eq!(
        raw.len(),
        n_w * n_plies,
        "raw slice length must be n_w * n_plies"
    );

    let mut deltas = vec![0i16; n_w * n_plies];
    for w in 0..n_w {
        let mut prev = 0i16;
        for p in 0..n_plies {
            let v = raw[p * n_w + w];
            deltas[w * n_plies + p] = if p == 0 { v } else { v.wrapping_sub(prev) };
            prev = v;
        }
    }

    let len = n_w * n_plies;
    let mut out = vec![0u8; len * 2];
    let (low, high) = out.split_at_mut(len);
    for (i, &d) in deltas.iter().enumerate() {
        let [lo, hi] = d.to_le_bytes();
        low[i] = lo;
        high[i] = hi;
    }
    out
}

/// Inverse of [`encode`]: reconstructs the ply-major raw slice from an encoded byte-plane blob.
///
/// Panics if `encoded.len() != 2 * n_w * n_plies`.
pub fn decode(encoded: &[u8], n_w: usize, n_plies: usize) -> Vec<i16> {
    let len = n_w * n_plies;
    assert_eq!(
        encoded.len(),
        len * 2,
        "encoded slice length must be 2 * n_w * n_plies"
    );

    let (low, high) = encoded.split_at(len);
    let mut raw = vec![0i16; len];
    for w in 0..n_w {
        let mut prev = 0i16;
        for p in 0..n_plies {
            let idx = w * n_plies + p;
            let delta = i16::from_le_bytes([low[idx], high[idx]]);
            let v = if p == 0 {
                delta
            } else {
                prev.wrapping_add(delta)
            };
            raw[p * n_w + w] = v;
            prev = v;
        }
    }
    raw
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trips_small_synthetic_data() {
        let n_w = 5;
        let n_plies = 3;
        let raw: Vec<i16> = vec![
            100,
            -200,
            300,
            0,
            i16::MAX, // ply 0
            101,
            -150,
            250,
            5,
            i16::MIN, // ply 1
            99,
            -400,
            300,
            -5,
            0, // ply 2
        ];
        let encoded = encode(&raw, n_w, n_plies);
        assert_eq!(encoded.len(), 2 * n_w * n_plies);
        assert_eq!(decode(&encoded, n_w, n_plies), raw);
    }

    #[test]
    fn round_trips_full_size_extreme_values() {
        // Alternating min/max stresses the wrapping delta arithmetic at the actual shipped shape.
        let raw: Vec<i16> = (0..N_W * N_PLIES)
            .map(|i| if i % 2 == 0 { i16::MIN } else { i16::MAX })
            .collect();
        let encoded = encode(&raw, N_W, N_PLIES);
        assert_eq!(decode(&encoded, N_W, N_PLIES), raw);
    }

    #[test]
    #[should_panic(expected = "raw slice length must be n_w * n_plies")]
    fn encode_rejects_wrong_length() {
        encode(&[0i16; 4], 5, 3);
    }

    #[test]
    #[should_panic(expected = "encoded slice length must be 2 * n_w * n_plies")]
    fn decode_rejects_wrong_length() {
        decode(&[0u8; 4], 5, 3);
    }
}
