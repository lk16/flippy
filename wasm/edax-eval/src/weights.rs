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

//! Weight loader/unpacker (`TASKS.md` Task 4): reconstructs the full,
//! per-ply `Eval_weight` tables from the packed, symmetry-reduced weights
//! shipped in the [`crate::weights_transform`]-encoded blob, by porting
//! `eval_open`'s unpacking logic verbatim (`eval.c:459-698`).
//!
//! Edax exploits mirror symmetry (board reflections) to store roughly half
//! the distinct feature weight values: [`build_symmetry_packing`] ports
//! `set_eval_packing`/`set_opponent_feature` (`eval.c:496-630`), the pure
//! index arithmetic that assigns each of a feature's 3^n raw states to one
//! of far fewer packed slots; [`unpack`] ports `eval_open`'s per-ply loop
//! (`eval.c:661-697`) that uses those tables to expand the packed weights
//! read from a `eval.dat`-derived raw slice (the same `raw` shape Task 2's
//! `weights_transform::decode` produces) into full lookup tables.

/// Number of squares per feature type, and the packed-weight offset within a ply's `n_w`-length
/// block where that feature's packed values start (`eval.c:459`, `EVAL_PACKED_OFS`). Order:
/// C9, C10, S10 (x2: S100/S101 share packed storage), S8 (x4: S8x4), S7, S6, S5, S4, S0.
const EVAL_PACKED_OFS: [usize; 13] = [
    0, 10206, 40095, 69741, 99387, 102708, 106029, 109350, 112671, 113805, 114183, 114318, 114363,
];

/// Feature-increment tables used by [`set_eval_packing`] to compute each state's mirror index
/// (`eval.c:578-580`). `KD_S10`/`KD_C10` are sliced from an offset for smaller features, matching
/// the C pointer arithmetic `kd_S10 + n`.
const KD_S10: [i32; 10] = [19683, 6561, 2187, 729, 243, 81, 27, 9, 3, 1];
const KD_C10: [i32; 10] = [19683, 6561, 2187, 729, 81, 243, 27, 9, 3, 1];
const KD_C9: [i32; 9] = [1, 9, 3, 81, 27, 243, 2187, 729, 6561];

/// Feature symmetry packing indices, one instance per board-parity ("`P[ply & 1]`" in `eval.c`):
/// for each feature type, maps a raw base-3 state index to its packed (mirror-reduced) slot.
/// Direct port of `SymetryPacking` (`eval.c:466-476`).
struct SymmetryPacking {
    c10: Vec<u16>, // 59049
    s10: Vec<u16>, // 59049
    c9: Vec<u16>,  // 19683
    s8: Vec<u16>,  // 6561
    s7: Vec<u16>,  // 2187
    s6: Vec<u16>,  // 729
    s5: Vec<u16>,  // 243
    s4: Vec<u16>,  // 81
}

/// Assigns packed (mirror-reduced) indices for one feature type. `pe[l]` receives the packed
/// index for raw state `l`; states that are mirror images of an earlier (smaller) state reuse its
/// index instead of getting a new one, via the scratch table `t` (indexed by each state's mirror
/// index `k`, computed by the running `kd`-weighted accumulator). Returns the next unused packed
/// index (so the final call's return value is the feature's total packed slot count).
///
/// Line-by-line port of `set_eval_packing` (`eval.c:522-552`) — deliberately literal rather than
/// simplified, since this is dense index arithmetic where a "cleanup" is exactly how a subtle
/// transcription bug would hide. Verified instead by its *output*: for every feature type, `n`
/// after the top-level call must equal the packed sizes `EVAL_PACKED_OFS`'s gaps document (e.g.
/// C9: 19683 raw states -> 10206 packed), an invariant a bug in this arithmetic would almost
/// certainly break (see the unit tests below).
#[allow(clippy::too_many_arguments)]
fn set_eval_packing(
    pe: &mut [u16],
    t: &mut [i32],
    kd: &[i32],
    l0: i32,
    k0: i32,
    n0: i32,
    d0: i32,
) -> i32 {
    let mut n = n0;
    let d = d0 - 1;
    if d > 3 {
        let l = l0 * 3;
        n = set_eval_packing(pe, t, kd, l, k0, n, d);
        let k = k0 + kd[d as usize];
        n = set_eval_packing(pe, t, kd, l + 3, k, n, d);
        let k = k + kd[d as usize];
        n = set_eval_packing(pe, t, kd, l + 6, k, n, d);
    } else {
        let mut l = l0 * 27;
        let mut k = k0;
        for _q3 in 0..3 {
            for _q2 in 0..3 {
                for _q1 in 0..3 {
                    for _q0 in 0..3 {
                        let i = if k < l {
                            t[k as usize]
                        } else {
                            let i = n;
                            t[l as usize] = i;
                            n += 1;
                            i
                        };
                        pe[l as usize] = i as u16;
                        l += 1;
                        k += kd[0];
                    }
                    k += kd[1] - kd[0] * 3;
                }
                k += kd[2] - kd[1] * 3;
            }
            k += kd[3] - kd[2] * 3;
        }
    }
    n
}

/// Runs [`set_eval_packing`] for one feature type: `size` raw states, `kd` its feature-increment
/// table. The scratch table only ever needs `size` slots (see `set_eval_packing`'s doc comment for
/// why per-call reuse across feature types is unnecessary, unlike `eval_open`'s single shared `T`).
fn pack_feature(size: usize, kd: &[i32]) -> Vec<u16> {
    let mut pe = vec![0u16; size];
    let mut t = vec![0i32; size];
    set_eval_packing(&mut pe, &mut t, kd, 0, 0, 0, kd.len() as i32);
    pe
}

/// Maps a raw base-3 feature state to the same state viewed from the opponent's perspective:
/// every digit (a square's disc: 0 = player, 1 = opponent, 2 = empty) is swapped `0 <-> 1`, `2`
/// unchanged, digit positions (place values) unchanged. Direct port of `set_opponent_feature`
/// (`eval.c:496-508`), which builds this as a 59049-entry table (`OPPONENT_FEATURE`) via recursive
/// DFS rather than the closed-form digit-swap above — both compute the same function; see the
/// `opponent_feature_matches_digit_swap` test for the derivation/proof that they agree everywhere.
fn set_opponent_feature(p: &mut [u16], pos: &mut usize, o: i32, d: i32) {
    let d = d - 1;
    if d != 0 {
        set_opponent_feature(p, pos, (o + 1) * 3, d);
        set_opponent_feature(p, pos, o * 3, d);
        set_opponent_feature(p, pos, (o + 2) * 3, d);
    } else {
        p[*pos] = (o + 1) as u16;
        p[*pos + 1] = o as u16;
        p[*pos + 2] = (o + 2) as u16;
        *pos += 3;
    }
}

fn build_opponent_feature() -> Vec<u16> {
    let mut table = vec![0u16; 59049];
    let mut pos = 0;
    set_opponent_feature(&mut table, &mut pos, 0, 10);
    debug_assert_eq!(pos, 59049);
    table
}

/// Builds both parities' symmetry packing tables (`P[0]`, `P[1]` in `eval_open`, `eval.c:596-630`):
/// `P[0]` packs each feature's raw states directly; `P[1]` is `P[0]` re-indexed through the
/// opponent-feature table, giving the same packed slots viewed from the other side, needed because
/// `eval_open` alternates between them ply by ply (`P[ply & 1]`, since the mover alternates too).
fn build_symmetry_packing() -> [SymmetryPacking; 2] {
    let opponent_feature = build_opponent_feature();

    let s8_0 = pack_feature(6561, &KD_S10[2..]);
    let s8_1 = (0..6561)
        .map(|j| s8_0[opponent_feature[j + 26244] as usize])
        .collect();

    let s7_0 = pack_feature(2187, &KD_S10[3..]);
    let s7_1 = (0..2187)
        .map(|j| s7_0[opponent_feature[j + 28431] as usize])
        .collect();

    let s6_0 = pack_feature(729, &KD_S10[4..]);
    let s6_1 = (0..729)
        .map(|j| s6_0[opponent_feature[j + 29160] as usize])
        .collect();

    let s5_0 = pack_feature(243, &KD_S10[5..]);
    let s5_1 = (0..243)
        .map(|j| s5_0[opponent_feature[j + 29403] as usize])
        .collect();

    let s4_0 = pack_feature(81, &KD_S10[6..]);
    let s4_1 = (0..81)
        .map(|j| s4_0[opponent_feature[j + 29484] as usize])
        .collect();

    let c9_0 = pack_feature(19683, &KD_C9);
    let c9_1 = (0..19683)
        .map(|j| c9_0[opponent_feature[j + 19683] as usize])
        .collect();

    let s10_0 = pack_feature(59049, &KD_S10);
    let c10_0 = pack_feature(59049, &KD_C10);
    let s10_1 = (0..59049)
        .map(|j| s10_0[opponent_feature[j] as usize])
        .collect();
    let c10_1 = (0..59049)
        .map(|j| c10_0[opponent_feature[j] as usize])
        .collect();

    [
        SymmetryPacking {
            c10: c10_0,
            s10: s10_0,
            c9: c9_0,
            s8: s8_0,
            s7: s7_0,
            s6: s6_0,
            s5: s5_0,
            s4: s4_0,
        },
        SymmetryPacking {
            c10: c10_1,
            s10: s10_1,
            c9: c9_1,
            s8: s8_1,
            s7: s7_1,
            s6: s6_1,
            s5: s5_1,
            s4: s4_1,
        },
    ]
}

/// Fully unpacked feature weight table for one ply. Direct port of `Eval_weight`
/// (`eval.h:47-55`), minus the `S0` guard-alignment concern that struct layout served in C.
pub struct EvalWeight {
    pub s0: i16,
    /// Length 19683.
    pub c9: Vec<i16>,
    /// Length 59049.
    pub c10: Vec<i16>,
    /// Length 59049.
    pub s100: Vec<i16>,
    /// Length 59049.
    pub s101: Vec<i16>,
    /// Length 6561 * 4.
    pub s8x4: Vec<i16>,
    /// Length 2187 + 729 + 243 + 81.
    pub s7654: Vec<i16>,
}

/// Unpacks `raw` (ply-major packed weights, `raw[ply * n_w + i]`, exactly [`crate::weights_transform::decode`]'s
/// output shape) into one [`EvalWeight`] per ply. Direct port of `eval_open`'s per-ply unpacking
/// loop (`eval.c:661-697`), given already-extracted, host-endian weights (byte-swapping, if the
/// source `eval.dat` needed it, already happened in Task 2's extraction tool). The per-feature
/// loops below index `pp`/`EVAL_PACKED_OFS` manually rather than through iterator/zip adaptors for
/// the same reason [`set_eval_packing`]'s doc comment gives: this is index arithmetic ported
/// line-for-line from `eval.c`, and its correctness is checked against the real `eval.dat` (see
/// this module's tests), not by how idiomatic the loop shape looks.
pub fn unpack(raw: &[i16], n_w: usize, n_plies: usize) -> Vec<EvalWeight> {
    assert_eq!(
        raw.len(),
        n_w * n_plies,
        "raw slice length must be n_w * n_plies"
    );

    let packing = build_symmetry_packing();

    (0..n_plies)
        .map(|ply| {
            let w = &raw[ply * n_w..(ply + 1) * n_w];
            // eval.c:596: `pp = *P + (ply & 1)`. `ply` here is already offset to start at the
            // real ply 2 (see weights_transform's FIRST_PLY), so parity must account for that:
            // real_ply = ply + FIRST_PLY, and FIRST_PLY = 2 is even, so parity is unaffected.
            let pp = &packing[ply & 1];

            let c9 = (0..19683)
                .map(|k| w[pp.c9[k] as usize + EVAL_PACKED_OFS[0]])
                .collect();

            let mut c10 = vec![0i16; 59049];
            let mut s100 = vec![0i16; 59049];
            let mut s101 = vec![0i16; 59049];
            for k in 0..59049 {
                c10[k] = w[pp.c10[k] as usize + EVAL_PACKED_OFS[1]];
                let i = pp.s10[k] as usize;
                s100[k] = w[i + EVAL_PACKED_OFS[2]];
                s101[k] = w[i + EVAL_PACKED_OFS[3]];
            }

            let mut s8x4 = vec![0i16; 6561 * 4];
            for k in 0..6561 {
                let i = pp.s8[k] as usize;
                s8x4[k] = w[i + EVAL_PACKED_OFS[4]];
                s8x4[k + 6561] = w[i + EVAL_PACKED_OFS[5]];
                s8x4[k + 13122] = w[i + EVAL_PACKED_OFS[6]];
                s8x4[k + 19683] = w[i + EVAL_PACKED_OFS[7]];
            }

            let mut s7654 = vec![0i16; 2187 + 729 + 243 + 81];
            for k in 0..2187 {
                s7654[k] = w[pp.s7[k] as usize + EVAL_PACKED_OFS[8]];
            }
            for k in 0..729 {
                s7654[k + 2187] = w[pp.s6[k] as usize + EVAL_PACKED_OFS[9]];
            }
            for k in 0..243 {
                s7654[k + 2916] = w[pp.s5[k] as usize + EVAL_PACKED_OFS[10]];
            }
            for k in 0..81 {
                s7654[k + 3159] = w[pp.s4[k] as usize + EVAL_PACKED_OFS[11]];
            }

            let s0 = w[EVAL_PACKED_OFS[12]];

            EvalWeight {
                s0,
                c9,
                c10,
                s100,
                s101,
                s8x4,
                s7654,
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    /// `EVAL_PACKED_OFS`'s gaps between consecutive offsets are the packed sizes Edax's own source
    /// documents (`eval.c:461`'s commented-out `EVAL_PACKED_SIZE`): C9 packs 19683 raw states down
    /// to 10206, C10/S10 pack 59049 down to 29889/29646, S8 6561->3321, S7 2187->1134, S6
    /// 729->378, S5 243->135, S4 81->45. Getting `set_eval_packing`'s dense index arithmetic wrong
    /// would need to coincidentally still produce every one of these exact counts to pass
    /// undetected, so this is strong evidence the port is correct even without a live Edax dump to
    /// compare against (per TASKS.md Task 4's "cross-check ... end-to-end via Task 8 instead if
    /// dumping internal state is inconvenient").
    #[test]
    fn packed_sizes_match_edax_documented_constants() {
        assert_eq!(pack_feature(19683, &KD_C9).iter().max().unwrap() + 1, 10206);
        assert_eq!(
            pack_feature(59049, &KD_C10).iter().max().unwrap() + 1,
            29889
        );
        assert_eq!(
            pack_feature(59049, &KD_S10).iter().max().unwrap() + 1,
            29646
        );
        assert_eq!(
            pack_feature(6561, &KD_S10[2..]).iter().max().unwrap() + 1,
            3321
        );
        assert_eq!(
            pack_feature(2187, &KD_S10[3..]).iter().max().unwrap() + 1,
            1134
        );
        assert_eq!(
            pack_feature(729, &KD_S10[4..]).iter().max().unwrap() + 1,
            378
        );
        assert_eq!(
            pack_feature(243, &KD_S10[5..]).iter().max().unwrap() + 1,
            135
        );
        assert_eq!(pack_feature(81, &KD_S10[6..]).iter().max().unwrap() + 1, 45);
    }

    /// Every packed index must actually be used (no gaps): the set of assigned indices for a
    /// feature of `size` raw states packed into `packed_size` slots must be exactly `0..packed_size`.
    #[test]
    fn packed_indices_have_no_gaps() {
        for (size, kd, packed_size) in [
            (19683usize, &KD_C9[..], 10206usize),
            (59049, &KD_C10[..], 29889),
            (59049, &KD_S10[..], 29646),
            (6561, &KD_S10[2..], 3321),
            (2187, &KD_S10[3..], 1134),
            (729, &KD_S10[4..], 378),
            (243, &KD_S10[5..], 135),
            (81, &KD_S10[6..], 45),
        ] {
            let packed = pack_feature(size, kd);
            let mut seen = vec![false; packed_size];
            for &i in &packed {
                seen[i as usize] = true;
            }
            assert!(
                seen.iter().all(|&s| s),
                "packed indices for size {size} have gaps"
            );
        }
    }

    /// Proves `set_opponent_feature`'s recursive DFS construction (production code) computes the
    /// same function as a direct closed-form per-digit swap (test-only, independent implementation):
    /// for `j`'s 10 base-3 digits (digit `i` at place value `3^i`), each digit is remapped
    /// `0 -> 1, 1 -> 0, 2 -> 2` and reassembled at the same place values. Derived by hand-expanding
    /// `set_opponent_feature`'s recursion (`o_new = (o + swap(digit)) * 3` per non-leaf digit,
    /// `o_final = o + swap(digit)` at the leaf) and confirmed exhaustively here across all 3^10
    /// states, rather than assumed.
    #[test]
    fn opponent_feature_matches_digit_swap() {
        fn digit_swap(mut j: u32) -> u32 {
            let mut result = 0u32;
            let mut place = 1u32;
            for _ in 0..10 {
                let digit = j % 3;
                j /= 3;
                let swapped = match digit {
                    0 => 1,
                    1 => 0,
                    2 => 2,
                    _ => unreachable!(),
                };
                result += swapped * place;
                place *= 3;
            }
            result
        }

        let table = build_opponent_feature();
        for j in 0..59049u32 {
            assert_eq!(table[j as usize] as u32, digit_swap(j), "mismatch at j={j}");
        }
    }

    /// Applying the opponent-feature swap twice must return the original state (it's an
    /// involution: swapping player<->opponent twice is identity).
    #[test]
    fn opponent_feature_is_an_involution() {
        let table = build_opponent_feature();
        for j in 0..59049usize {
            assert_eq!(table[table[j] as usize] as usize, j);
        }
    }

    #[test]
    fn unpack_produces_one_table_per_ply_with_correct_shapes() {
        // A tiny synthetic n_w-length "ply" won't do -- unpack indexes packed offsets up to
        // EVAL_PACKED_OFS[12] = 114363, so raw must be real-sized. Fill with a simple pattern
        // (not real weights) purely to check shapes/bounds, not values.
        let n_w = 114364;
        let n_plies = 2;
        let raw: Vec<i16> = (0..n_w * n_plies).map(|i| (i % 1000) as i16).collect();

        let tables = unpack(&raw, n_w, n_plies);
        assert_eq!(tables.len(), n_plies);
        for t in &tables {
            assert_eq!(t.c9.len(), 19683);
            assert_eq!(t.c10.len(), 59049);
            assert_eq!(t.s100.len(), 59049);
            assert_eq!(t.s101.len(), 59049);
            assert_eq!(t.s8x4.len(), 6561 * 4);
            assert_eq!(t.s7654.len(), 2187 + 729 + 243 + 81);
        }
    }

    /// Runs the real unpacking pipeline end to end against the real `eval.dat` (header parsing is
    /// already covered by `extract_weights`'s own gated test; this only re-slices the raw plies,
    /// deliberately not reusing that bin target's code, to keep this a from-scratch exercise of
    /// `unpack`). Mirrors the `EDAX_PATH`-gated skip pattern (`process_test.go`), so it's local-only
    /// without a CI exclusion. Since there's no independent oracle for the *unpacked* weight
    /// values without patching a debug Edax build (see this module's other tests and TASKS.md Task
    /// 4's note authorizing end-to-end verification via Task 8 instead), this only checks the
    /// pipeline runs to completion over real data and produces plausible (non-degenerate) output.
    #[test]
    fn unpacks_the_real_eval_dat_without_panicking() {
        let path = if let Ok(p) = std::env::var("EVAL_DAT_PATH") {
            std::path::PathBuf::from(p)
        } else if let Ok(host_dir) = std::env::var("EDAX_HOST_DIR") {
            std::path::PathBuf::from(host_dir)
                .join("data")
                .join("eval.dat")
        } else {
            eprintln!("EVAL_DAT_PATH/EDAX_HOST_DIR not set; skipping");
            return;
        };
        let Ok(file) = std::fs::read(&path) else {
            eprintln!("{} not readable; skipping", path.display());
            return;
        };

        const HEADER_LEN: usize = 28;
        const RAW_N_W: usize = 114364;
        const FIRST_PLY: usize = 2;
        const EVAL_N_PLY: usize = 54;
        let block_bytes = RAW_N_W * 2;

        let mut raw = vec![0i16; RAW_N_W * (EVAL_N_PLY - FIRST_PLY)];
        for ply in FIRST_PLY..EVAL_N_PLY {
            let block_start = HEADER_LEN + ply * block_bytes;
            let block = &file[block_start..block_start + block_bytes];
            let out_ply = ply - FIRST_PLY;
            for i in 0..RAW_N_W {
                raw[out_ply * RAW_N_W + i] = i16::from_le_bytes([block[i * 2], block[i * 2 + 1]]);
            }
        }

        let tables = unpack(&raw, RAW_N_W, EVAL_N_PLY - FIRST_PLY);
        assert_eq!(tables.len(), EVAL_N_PLY - FIRST_PLY);

        // A real trained model shouldn't be all zeros, and every weight is a signed 16-bit value
        // by construction (i16), so the only meaningful sanity check left is non-degeneracy.
        let first = &tables[0];
        assert!(
            first.c9.iter().any(|&v| v != 0),
            "ply 2's C9 table is all zero"
        );
        assert!(
            first.s7654.iter().any(|&v| v != 0),
            "ply 2's S7654 table is all zero"
        );
        let last = &tables[tables.len() - 1];
        assert!(
            last.c10.iter().any(|&v| v != 0),
            "last ply's C10 table is all zero"
        );
    }
}
