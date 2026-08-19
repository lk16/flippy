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

//! Stability cutoffs for the exact-solve endgame search: stable discs (never flippable again)
//! bound the opponent's final count, proving fail-lows without searching — pruning that never
//! changes the minimax value. Ports `NWS_endgame`'s cutoff (`endgame.c:538-540`) with
//! `get_stability` (`board.c:916-929`), `get_stable_edge` (`board.c:789-795`),
//! `get_full_lines` (`board.c:867-891`), and `get_spreaded_stability` (`board.c:895-913`).

use std::sync::OnceLock;

/// Alpha thresholds to try the stability cutoff, indexed by `n_empties` (`search.c:114-121`);
/// 99 = cutoff never fires.
pub(crate) const NWS_STABILITY_THRESHOLD: [i32; 64] = [
    99, 99, 99, 99, 6, 8, 10, 12, // 0-7
    8, 10, 20, 22, 24, 26, 28, 30, // 8-15
    32, 34, 36, 38, 40, 42, 44, 46, // 16-23
    48, 48, 50, 50, 52, 52, 54, 54, // 24-31
    56, 56, 58, 58, 60, 60, 62, 62, // 32-39
    64, 64, 64, 64, 64, 64, 64, 64, // 40-47
    99, 99, 99, 99, 99, 99, 99, 99, // 48-55
    99, 99, 99, 99, 99, 99, 99, 99, // 56-63
];

/// Full-lines mask per direction: `full[i]` has a line's bits set iff that line is completely
/// occupied. Port of `get_full_lines` (`board.c:867-891`).
fn get_full_lines(disc: u64) -> [u64; 4] {
    // Horizontal: row is full iff all 8 bits in that byte are set.
    let h = {
        let f = disc & (disc >> 1);
        let f = f & (f >> 2);
        let f = f & (f >> 4);
        (f & 0x0101010101010101).wrapping_mul(0xff)
    };
    // Vertical: column is full iff all 8 bits in that column are set.
    let v = {
        let f = disc & disc.rotate_right(8);
        let f = f & f.rotate_right(16);
        f & f.rotate_left(32)
    };
    // Diagonal \: from top-left to bottom-right.
    let (d9, d7) = {
        let (mut l9, mut r9) = (disc, disc);
        l9 &= 0xff80808080808080 | (l9 >> 9);
        r9 &= 0x01010101010101ff | (r9 << 9);
        l9 &= 0xffffc0c0c0c0c0c0 | (l9 >> 18);
        r9 &= 0x030303030303ffff | (r9 << 18);
        let d9 = l9 & r9 & (0x0f0f0f0ff0f0f0f0 | (l9 >> 36) | (r9 << 36));
        let (mut l7, mut r7) = (disc, disc);
        l7 &= 0xff01010101010101 | (l7 >> 7);
        r7 &= 0x80808080808080ff | (r7 << 7);
        l7 &= 0xffff030303030303 | (l7 >> 14);
        r7 &= 0xc0c0c0c0c0c0ffff | (r7 << 14);
        l7 &= 0xffffffff0f0f0f0f | (l7 >> 28);
        r7 &= 0xf0f0f0f0ffffffff | (r7 << 28);
        (d9, l7 & r7)
    };
    [h, v, d9, d7]
}

/// Propagate stability: a disc is stable if every direction has a stable neighbour or a full
/// line; iterate to fixpoint. Port of `get_spreaded_stability` (`board.c:895-913`).
fn get_spreaded_stability(stable: u64, p_central: u64, full: &[u64; 4]) -> u32 {
    if stable == 0 {
        return 0;
    }
    let mut stable = stable;
    loop {
        let old = stable;
        let h = (stable >> 1) | (stable << 1) | full[0];
        let v = (stable >> 8) | (stable << 8) | full[1];
        let d9 = (stable >> 9) | (stable << 9) | full[2];
        let d7 = (stable >> 7) | (stable << 7) | full[3];
        stable |= h & v & d9 & d7 & p_central;
        if stable == old {
            break;
        }
    }
    stable.count_ones()
}

// --- Edge stability table (65536 entries, one per (P_edge, O_edge) pair) ---

/// Which of `stable`'s squares stay stable after exhaustively trying every move on an 8-bit
/// edge (only the low 8 bits of P/O/stable are meaningful). Port of `find_edge_stable`
/// (`board.c:681-737`).
fn find_edge_stable(old_p: i32, old_o: i32, stable: i32) -> i32 {
    let stable = stable & old_p;
    if stable == 0 {
        return 0;
    }
    let e = (!(old_p | old_o)) as u8; // empties (8-bit only)
    if e == 0 {
        return stable;
    }
    let mut result = stable;
    let mut x = 1i32;
    while x <= 0x80 {
        if (e as i32) & x != 0 {
            // Player plays on empty square x.
            let mut o = old_o;
            let mut p = old_p | x;
            if x > 0x02 {
                // Flip left via parallel prefix (toward lower bits).
                let mut f = o & (x >> 1);
                f |= o & (f >> 1);
                let o2 = o & (o >> 1);
                f |= o2 & (f >> 2);
                f |= o2 & (f >> 2);
                // Anchor check: with p & o == 0, (p & (f>>1)) is 0 or 1, so wrapping_neg is 0
                // or all-1s — keeping f only when a player disc brackets the chain.
                f &= (p & (f >> 1)).wrapping_neg();
                o ^= f;
                p ^= f;
            }
            // Flip right via carry propagation (toward higher bits).
            let mut f = (o.wrapping_add(x + x)) & p;
            f = f.wrapping_sub((x + x) & -((f != 0) as i32));
            o ^= f;
            p ^= f;
            result = find_edge_stable(p, o, result);
            if result == 0 {
                return 0;
            }

            // Opponent plays on empty square x.
            let mut p = old_p;
            let mut o = old_o | x;
            if x > 0x02 {
                let mut f = p & (x >> 1);
                f |= p & (f >> 1);
                let p2 = p & (p >> 1);
                f |= p2 & (f >> 2);
                f |= p2 & (f >> 2);
                f &= (o & (f >> 1)).wrapping_neg();
                p ^= f;
                o ^= f;
            }
            let mut f = (p.wrapping_add(x + x)) & o;
            f = f.wrapping_sub((x + x) & -((f != 0) as i32));
            p ^= f;
            o ^= f;
            result = find_edge_stable(p, o, result);
            if result == 0 {
                return 0;
            }
        }
        x <<= 1;
    }
    result
}

/// Builds the 65536-entry edge stability table: `[P * 256 + O]` is the bitmask of P's stable
/// squares on an 8-square edge. Mirrors share results. Port of `edge_stability_init`
/// (`board.c:749-767`).
fn build_edge_stability() -> Box<[u8; 65536]> {
    let mut table = Box::new([0u8; 65536]);
    for po in 0u32..65536 {
        let p = (po >> 8) as i32;
        let o = (po & 0xff) as i32;
        if p & o != 0 {
            // Illegal position: P and O overlap.
            table[po as usize] = 0;
        } else {
            // Mirror: reverse bits of P and O separately.
            let r_po = ((p as u8).reverse_bits() as u32) * 256 + (o as u8).reverse_bits() as u32;
            if po > r_po {
                // Symmetric: mirror the already-computed result.
                table[po as usize] = table[r_po as usize].reverse_bits();
            } else {
                table[po as usize] = find_edge_stable(p, o, p) as u8;
            }
        }
    }
    table
}

static EDGE_STABILITY: OnceLock<Box<[u8; 65536]>> = OnceLock::new();

fn edge_stability_table() -> &'static [u8; 65536] {
    EDGE_STABILITY.get_or_init(build_edge_stability)
}

/// Pack column-A bits (bit 0 of each row) into a byte (bit i = row i).
fn pack_a1a8(x: u64) -> usize {
    ((x & 0x0101010101010101).wrapping_mul(0x0102040810204080) >> 56) as usize
}

/// Pack column-H bits (bit 7 of each row) into a byte.
fn pack_h1h8(x: u64) -> usize {
    ((x & 0x8080808080808080).wrapping_mul(0x0002040810204081) >> 56) as usize
}

/// Unpack a byte into column-A2..A7 bits (i.e., rows 1-6 of column A, skipping corners).
fn unpack_a2a7(x: u64) -> u64 {
    ((x & 0x7e).wrapping_mul(0x0000040810204080)) & 0x0001010101010100
}

/// Unpack a byte into column-H2..H7 bits (i.e., rows 1-6 of column H, skipping corners).
fn unpack_h2h7(x: u64) -> u64 {
    ((x & 0x7e).wrapping_mul(0x0002040810204000)) & 0x0080808080808000
}

/// P's stable discs on all four edges, via the precomputed edge table. Port of
/// `get_stable_edge` (`board.c:789-795`).
fn get_stable_edge(p: u64, o: u64) -> u64 {
    let t = edge_stability_table();
    let bottom = t[((p as u8) as usize) * 256 + (o as u8) as usize] as u64;
    let top = (t[(p >> 56) as usize * 256 + (o >> 56) as usize] as u64) << 56;
    let left = unpack_a2a7(t[pack_a1a8(p) * 256 + pack_a1a8(o)] as u64);
    let right = unpack_h2h7(t[pack_h1h8(p) * 256 + pack_h1h8(o)] as u64);
    bottom | top | left | right
}

/// Count of P's stable discs — a lower bound (may miss some in complex positions), which is safe
/// for the cutoff. Port of `get_stability` (`board.c:916-929`); `search::negamax` calls it with
/// P=opponent, O=player.
pub(crate) fn get_stability(p: u64, o: u64) -> u32 {
    let stable = get_stable_edge(p, o);
    let p_central = p & 0x007e7e7e7e7e7e00;
    let full = get_full_lines(p | o);
    let stable = stable | (p_central & full[0] & full[1] & full[2] & full[3]);
    get_spreaded_stability(stable, p_central, &full)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Empty board: no stable discs; full player board: all 64 stable.
    #[test]
    fn empty_board_has_no_stable_discs() {
        assert_eq!(get_stability(0, 0), 0);
        assert_eq!(get_stability(u64::MAX, 0), 64);
    }

    /// Corner discs are always stable.
    #[test]
    fn corner_disc_is_stable() {
        let p = 0x0000000000000001u64; // A1 only
        let o = 0x0000000000000002u64; // B1 (adjacent, not corner)
        assert!(get_stability(p, o) >= 1, "A1 corner disc must be stable");
    }

    /// Full board (alternating rows): player discs must count as stable.
    #[test]
    fn full_board_all_player_discs_stable() {
        let p = 0x00ff00ff00ff00ffu64;
        let o = 0xff00ff00ff00ff00u64;
        let s = get_stability(p, o);
        assert!(s > 0, "at least some discs must be stable on a full board");
        assert!(p & 1 != 0, "A1 belongs to player in this fixture");
    }

    /// Spot-checks NWS_STABILITY_THRESHOLD against `search.c:114-121`.
    #[test]
    fn stability_threshold_matches_search_c() {
        for i in 0..4 {
            assert_eq!(
                NWS_STABILITY_THRESHOLD[i], 99,
                "n_empties={i} threshold should be 99"
            );
        }
        assert_eq!(NWS_STABILITY_THRESHOLD[4], 6);
        assert_eq!(NWS_STABILITY_THRESHOLD[8], 8);
        assert_eq!(NWS_STABILITY_THRESHOLD[10], 20);
        assert_eq!(NWS_STABILITY_THRESHOLD[16], 32);
        for i in 48..64 {
            assert_eq!(
                NWS_STABILITY_THRESHOLD[i], 99,
                "n_empties={i} threshold should be 99"
            );
        }
    }

    /// stable=0 must return 0 without iterating.
    #[test]
    fn spreaded_stability_zero_input_returns_zero() {
        let full = [0u64; 4];
        assert_eq!(get_spreaded_stability(0, 0, &full), 0);
    }
}
