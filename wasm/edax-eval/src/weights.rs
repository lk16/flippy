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

//! Weight loader/unpacker: reconstructs the full per-ply `Eval_weight` tables from the packed,
//! mirror-symmetry-reduced weights in the [`crate::weights_transform`]-encoded blob.
//! [`build_symmetry_packing`] ports `set_eval_packing`/`set_opponent_feature`
//! (`eval.c:496-630`); [`unpack`] ports `eval_open`'s per-ply loop (`eval.c:661-697`).

/// Packed-weight offset per feature type within a ply's `n_w`-length block (`eval.c:459`,
/// `EVAL_PACKED_OFS`). Order: C9, C10, S10 x2, S8 x4, S7, S6, S5, S4, S0.
const EVAL_PACKED_OFS: [usize; 13] = [
    0, 10206, 40095, 69741, 99387, 102708, 106029, 109350, 112671, 113805, 114183, 114318, 114363,
];

/// Feature-increment tables for mirror-index computation (`eval.c:578-580`); sliced from an
/// offset for smaller features, matching the C pointer arithmetic `kd_S10 + n`.
const KD_S10: [i32; 10] = [19683, 6561, 2187, 729, 243, 81, 27, 9, 3, 1];
const KD_C10: [i32; 10] = [19683, 6561, 2187, 729, 81, 243, 27, 9, 3, 1];
const KD_C9: [i32; 9] = [1, 9, 3, 81, 27, 243, 2187, 729, 6561];

/// Per-parity symmetry packing indices (`P[ply & 1]` in `eval.c`): raw base-3 state →
/// mirror-reduced packed slot. Port of `SymetryPacking` (`eval.c:466-476`).
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

/// Assigns packed (mirror-reduced) indices for one feature type: `pe[l]` gets raw state `l`'s
/// packed index, mirror images reuse the earlier state's index via scratch `t`; returns the next
/// unused packed index. Line-by-line port of `set_eval_packing` (`eval.c:522-552`), deliberately
/// literal — verified by output instead (packed sizes must match `EVAL_PACKED_OFS`'s gaps, see
/// the tests).
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

/// Runs [`set_eval_packing`] for one feature type of `size` raw states.
fn pack_feature(size: usize, kd: &[i32]) -> Vec<u16> {
    let mut pe = vec![0u16; size];
    let mut t = vec![0i32; size];
    set_eval_packing(&mut pe, &mut t, kd, 0, 0, 0, kd.len() as i32);
    pe
}

/// Maps a raw base-3 state to the opponent's view: each digit swapped `0 <-> 1`, `2` unchanged.
/// Port of `set_opponent_feature` (`eval.c:496-508`); the digit-swap test proves the recursive
/// construction equals the closed form.
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

/// Builds both parities' packing tables (`P[0]`/`P[1]`, `eval.c:596-630`): `P[0]` packs raw
/// states directly, `P[1]` re-indexes through the opponent-feature table; `eval_open` alternates
/// between them ply by ply.
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

/// Fully unpacked feature weight table for one ply. Port of `Eval_weight` (`eval.h:47-55`).
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

/// Unpacks `raw` (ply-major, [`crate::weights_transform::decode`]'s host-endian output shape)
/// into one [`EvalWeight`] per ply. Port of `eval_open`'s per-ply loop (`eval.c:661-697`);
/// indexing kept line-for-line literal for the same reason as [`set_eval_packing`].
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
            // eval.c:596: `pp = *P + (ply & 1)`. `ply` 0 here is real ply 2 (even), so parity
            // carries over unaffected.
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

    /// Packed sizes must match `eval.c:461`'s documented `EVAL_PACKED_SIZE` constants — a bug in
    /// `set_eval_packing`'s index arithmetic would almost certainly break these exact counts.
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

    /// Every packed index must be used: assigned indices must be exactly `0..packed_size`.
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

    /// Proves `set_opponent_feature`'s recursive construction equals an independent closed-form
    /// per-digit swap (`0 -> 1, 1 -> 0, 2 -> 2`), exhaustively over all 3^10 states.
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

    /// Swapping player<->opponent twice must be identity.
    #[test]
    fn opponent_feature_is_an_involution() {
        let table = build_opponent_feature();
        for j in 0..59049usize {
            assert_eq!(table[table[j] as usize] as usize, j);
        }
    }

    #[test]
    fn unpack_produces_one_table_per_ply_with_correct_shapes() {
        // unpack indexes packed offsets up to EVAL_PACKED_OFS[12] = 114363, so raw must be
        // real-sized; the pattern only checks shapes/bounds, not values.
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

    /// Runs the real unpacking pipeline against the real `eval.dat` (skipped when unavailable,
    /// mirroring the `EDAX_PATH` gate pattern); with no independent oracle for unpacked values,
    /// only checks completion and non-degenerate output.
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
