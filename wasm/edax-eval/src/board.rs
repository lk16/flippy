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

//! Bitboard representation and move generation, ported from Edax's `board.c`
//! (`TASKS.md` Task 3). Only the portable, non-SIMD code paths are ported —
//! `board_sse.c`/`board_mmx.c` and the vectorized `flip_*.c` variants are
//! performance-only alternates for the same logic (Edax's own `flip_slow.c`,
//! ported as [`Board::get_flip`] below, exists specifically to assert that
//! equivalence in Edax's own test suite).
//!
//! Squares are numbered exactly as Edax's `const.h` enum: `A1 = 0`, `H1 = 7`,
//! `A2 = 8`, ..., `H8 = 63` (bit `x` of a bitboard is square `x`, row-major).
//! A [`Board`] is always mover-relative, matching Edax's `player`/`opponent`
//! fields (`bit.h:147`) rather than a black/white representation — this is
//! what later tasks' eval/search logic is built against.

/// A mover-relative board: `player` is the side to move, `opponent` the other side.
/// Mirrors Edax's `Board` struct (`bit.h:147-149`).
#[derive(Copy, Clone, PartialEq, Eq, Debug)]
pub struct Board {
    pub player: u64,
    pub opponent: u64,
}

/// The standard Othello starting position, black to move (`board_init`, `board.c`).
pub const START: Board = Board {
    player: 0x0000_0008_1000_0000,
    opponent: 0x0000_0010_0800_0000,
};

/// Column-edge mask excluding the A and H files, used to stop horizontal/diagonal flip
/// propagation from wrapping across board edges (`board.c:562`: `O & 0x7e7e7e7e7e7e7e7e`).
const NOT_AH_FILE: u64 = 0x7e7e_7e7e_7e7e_7e7e;

impl Board {
    /// Propagates flips one direction at a time across the whole board, the way `get_moves` finds
    /// every legal move without testing squares individually. Direct port of `get_some_moves`'s
    /// portable "sequential algorithm" branch (`board.c:536-546`); `mask` is the opponent bitboard
    /// with edge squares excluded (for horizontal/diagonal directions) or the raw opponent
    /// bitboard (for vertical, which never wraps).
    #[inline]
    fn get_some_moves(p: u64, mask: u64, dir: u32) -> u64 {
        let mut flip = (p.wrapping_shl(dir) | p.wrapping_shr(dir)) & mask;
        for _ in 0..5 {
            flip |= (flip.wrapping_shl(dir) | flip.wrapping_shr(dir)) & mask;
        }
        flip.wrapping_shl(dir) | flip.wrapping_shr(dir)
    }

    /// All legal moves for `self.player`, as a bitboard. Direct port of `get_moves`
    /// (`board.c:560-580`), skipping its SSE/MMX dispatch (portable path only).
    #[inline]
    pub fn get_moves(&self) -> u64 {
        let om = self.opponent & NOT_AH_FILE;
        let moves = Self::get_some_moves(self.player, om, 1) // horizontal
            | Self::get_some_moves(self.player, self.opponent, 8) // vertical
            | Self::get_some_moves(self.player, om, 7) // diagonal
            | Self::get_some_moves(self.player, om, 9); // diagonal
        moves & !(self.player | self.opponent)
    }

    /// Whether `self.player` has any legal move. Direct port of `can_move` (`board.c:606-620`,
    /// portable path).
    pub fn can_move(&self) -> bool {
        self.get_moves() != 0
    }

    /// The bitboard of opponent discs that playing at `x` (`0..64`, an empty square) would flip.
    ///
    /// Uses bitboard propagation with compile-time-constant shift amounts — the same technique
    /// `get_moves` uses via `get_some_moves`, applied to a single starting square rather than the
    /// whole player bitboard. Each direction propagates through consecutive opponent bits until it
    /// either runs out of opponent or hits an empty/player square; a player bracket at the far end
    /// confirms the flip. The NOT_AH_FILE mask on the opponent prevents horizontal and diagonal
    /// propagation from wrapping across the A↔H file boundary; vertical shifts (±8) never wrap
    /// columns, so they use the raw opponent bitboard. This replaces the original `flip_slow.c`
    /// port's loop-per-direction approach (variable shifts + per-iteration edge checks = two
    /// avoidable bottlenecks for the compiler).
    ///
    /// Precondition: `x < 64`. Passing is a distinct operation (`Board::pass`), not handled here.
    pub fn get_flip(&self, x: u32) -> u64 {
        debug_assert!(x < 64, "get_flip called with out-of-range square {x}");
        let bit = 1u64 << x;
        let p = self.player;
        // Opponent masked to non-A/H columns: prevents horizontal/diagonal propagation from
        // wrapping (same mask get_moves uses). Vertical (shift=8) never wraps columns, so it
        // uses the raw opponent bitboard.
        let om = self.opponent & NOT_AH_FILE;
        let ov = self.opponent;

        let mut flipped = 0u64;

        // East (+1): propagate right through column-masked opponent; bracket must be further right.
        let mut gen = (bit << 1) & om;
        if gen != 0 {
            gen |= (gen << 1) & om;
            gen |= (gen << 1) & om;
            gen |= (gen << 1) & om;
            gen |= (gen << 1) & om;
            gen |= (gen << 1) & om;
            if (gen << 1) & p != 0 {
                flipped |= gen;
            }
        }

        // West (-1)
        let mut gen = (bit >> 1) & om;
        if gen != 0 {
            gen |= (gen >> 1) & om;
            gen |= (gen >> 1) & om;
            gen |= (gen >> 1) & om;
            gen |= (gen >> 1) & om;
            gen |= (gen >> 1) & om;
            if (gen >> 1) & p != 0 {
                flipped |= gen;
            }
        }

        // South (+8): higher bit index = further down the board.
        let mut gen = (bit << 8) & ov;
        if gen != 0 {
            gen |= (gen << 8) & ov;
            gen |= (gen << 8) & ov;
            gen |= (gen << 8) & ov;
            gen |= (gen << 8) & ov;
            gen |= (gen << 8) & ov;
            if (gen << 8) & p != 0 {
                flipped |= gen;
            }
        }

        // North (-8)
        let mut gen = (bit >> 8) & ov;
        if gen != 0 {
            gen |= (gen >> 8) & ov;
            gen |= (gen >> 8) & ov;
            gen |= (gen >> 8) & ov;
            gen |= (gen >> 8) & ov;
            gen |= (gen >> 8) & ov;
            if (gen >> 8) & p != 0 {
                flipped |= gen;
            }
        }

        // SE (+9)
        let mut gen = (bit << 9) & om;
        if gen != 0 {
            gen |= (gen << 9) & om;
            gen |= (gen << 9) & om;
            gen |= (gen << 9) & om;
            gen |= (gen << 9) & om;
            gen |= (gen << 9) & om;
            if (gen << 9) & p != 0 {
                flipped |= gen;
            }
        }

        // NW (-9)
        let mut gen = (bit >> 9) & om;
        if gen != 0 {
            gen |= (gen >> 9) & om;
            gen |= (gen >> 9) & om;
            gen |= (gen >> 9) & om;
            gen |= (gen >> 9) & om;
            gen |= (gen >> 9) & om;
            if (gen >> 9) & p != 0 {
                flipped |= gen;
            }
        }

        // SW (+7)
        let mut gen = (bit << 7) & om;
        if gen != 0 {
            gen |= (gen << 7) & om;
            gen |= (gen << 7) & om;
            gen |= (gen << 7) & om;
            gen |= (gen << 7) & om;
            gen |= (gen << 7) & om;
            if (gen << 7) & p != 0 {
                flipped |= gen;
            }
        }

        // NE (-7)
        let mut gen = (bit >> 7) & om;
        if gen != 0 {
            gen |= (gen >> 7) & om;
            gen |= (gen >> 7) & om;
            gen |= (gen >> 7) & om;
            gen |= (gen >> 7) & om;
            gen |= (gen >> 7) & om;
            if (gen >> 7) & p != 0 {
                flipped |= gen;
            }
        }

        flipped
    }

    /// Plays `x` (`0..64`, must be a legal move — i.e. present in [`Board::get_moves`]) and
    /// returns the resulting board, player/opponent swapped to the new side to move. Direct port
    /// of `board_update`'s portable path (`board.c:390-408`).
    pub fn play(&self, x: u32) -> Board {
        let flipped = self.get_flip(x);
        Board {
            opponent: self.player ^ (flipped | (1u64 << x)),
            player: self.opponent ^ flipped,
        }
    }

    /// Swaps player/opponent without flipping any discs, for when `self.player` has no legal
    /// move. Direct port of `board_pass` (`board.c:452-456`).
    pub fn pass(&self) -> Board {
        Board {
            player: self.opponent,
            opponent: self.player,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn start_position_has_four_legal_moves() {
        assert_eq!(START.get_moves().count_ones(), 4);
        assert!(START.can_move());
    }

    #[test]
    fn every_opening_move_flips_exactly_one_disc_and_swaps_sides() {
        for x in 0..64 {
            if START.get_moves() & (1u64 << x) == 0 {
                continue;
            }
            let flipped = START.get_flip(x);
            assert_eq!(
                flipped.count_ones(),
                1,
                "opening move {x} should flip exactly one disc"
            );

            let after = START.play(x);
            // The played square and the flipped disc(s) both end up belonging to the mover, i.e.
            // to `after.opponent` (player/opponent swap on the returned board).
            assert_eq!(after.opponent, START.player | flipped | (1u64 << x));
            assert_eq!(after.player, START.opponent & !flipped);
            // Total disc count grows by exactly one (a move places a disc, never removes one).
            let before_discs = (START.player | START.opponent).count_ones();
            let after_discs = (after.player | after.opponent).count_ones();
            assert_eq!(after_discs, before_discs + 1);
        }
    }

    #[test]
    fn pass_swaps_player_and_opponent_without_changing_discs() {
        let b = Board {
            player: 0x1,
            opponent: 0x2,
        };
        assert_eq!(
            b.pass(),
            Board {
                player: 0x2,
                opponent: 0x1
            }
        );
    }

    /// A frozen snapshot of a real forced-pass game (PlayOK `playok_normal.pgn`), one board per
    /// ply, in the same `black`/`white`/turn encoding and exact values as
    /// `static/test/fixtures.js`'s `FORCED_PASS_BOARDS` (there generated from, and cross-verified
    /// against, `internal/othello`'s and `static/board.js`'s independent move-generation ports —
    /// see `docs/project.md`). Regenerate the same way: `go run` a throwaway main over
    /// `internal/othello/testdata/pgn/playok_normal.pgn`.
    ///
    /// Plies 55 and 57 are forced passes (the side to move has none); ply 62 is game over.
    const FORCED_PASS_BOARDS: [&str; 63] = [
        "00000008100000000000001008000000-b",
        "00000038100000000000000008000000-w",
        "00000030100000000000080808000000-b",
        "000000301c0000000000080800000000-w",
        "00000030140000000000080808080000-b",
        "000000301c0400000000080800080000-w",
        "000000201c0400000000081820080000-b",
        "0000003c1c0400000000080020080000-w",
        "0000003c1c00000000000800200e0000-b",
        "0000003c1c0c04000000080020020000-w",
        "0000003c1c00040000000800201e0000-b",
        "0000003c1c080c000000080020160000-w",
        "0000003818080c0000000c0424160000-b",
        "000000381e0c0c0000000c0420120000-w",
        "000000381c0c0c0000000c0622120000-b",
        "000000381c1c2c0000000c0622020000-w",
        "0000003818142c0000000c06260a1000-b",
        "0000003838342c0000000c06060a1000-w",
        "000000383830280000000c06060e1404-b",
        "000000383830381000000c06060e0404-w",
        "000000383830301000000c06060e0c0c-b",
        "000408383830301000000406060e0c0c-w",
        "000408383830200000000406060e1c3c-b",
        "00040e3c3830200000000002060e1c3c-w",
        "00000a383830200004040406060e1c3c-b",
        "00000a3f3830200004040400060e1c3c-w",
        "00000a3d3830200004040402070e1c3c-b",
        "00000a3d3b3f20000404040204001c3c-w",
        "0000023533372000040c0c0a0c081c3c-b",
        "00001e3d37372000040c000208081c3c-w",
        "00001e3d37270000040c000208183c7c-b",
        "00001f3f37270000040c000008183c7c-w",
        "00001f3f17270000040c004028183c7c-b",
        "00001fff17270000040c000028183c7c-w",
        "00001fdf07270000040c402038183c7c-b",
        "00001fff7f270000040c400000183c7c-w",
        "00001fbf5f270000040cc04020183c7c-b",
        "00001fbf7f670000040cc04000183c7c-w",
        "00001f3f7f270000040cc0c080583c7c-b",
        "00001f3f7fe70000040cc0c080183c7c-w",
        "00000f2f6fe70000041cd0d090183c7c-b",
        "00003f3f6fe70000041cc0c090183c7c-w",
        "0000372f4f270000041cc8d0b0d8bc7c-b",
        "1018372f4f2700000404c8d0b0d8bc7c-w",
        "1000172f4f2700000c1ce8d0b0d8bc7c-b",
        "1e00172f4f270000001ce8d0b0d8bc7c-w",
        "1e0013274f270000001eecd8b0d8bc7c-b",
        "3e101b274f270000000ee4d8b0d8bc7c-w",
        "3e000b274f270000003ef4d8b0d8bc7c-b",
        "7e201b2f4f270000001ee4d0b0d8bc7c-w",
        "7e20192d45210000001ee6d2badebe7c-b",
        "7e20192d45230100001ee6d2badcbe7c-w",
        "7e20192d45030100001ee6d2bafcfe7c-b",
        "7e20192d55234180001ee6d2aadcbe7c-w",
        "7e00192d55234180007ee6d2aadcbe7c-b",
        "7e011b2d55234180007ee4d2aadcbe7c-w", // 55: forced pass (white to move, no moves)
        "7e011b2d55234180007ee4d2aadcbe7c-b", // 56: post-pass (same discs, black to move)
        "7f031f2d55234180007ce0d2aadcbe7c-w", // 57: forced pass (white to move, no moves)
        "7f031f2d55234180007ce0d2aadcbe7c-b", // 58: post-pass
        "7fffdfadd5a3c180000020522a5c3e7c-w",
        "7fbfdfadd5a3c180804020522a5c3e7c-b",
        "7fbfdfadd5abc7fe804020522a543800-w",
        "7fbfdfadd5abc5fe804020522a543a01-b", // 62: final, game over (neither can move)
    ];

    /// Independent, deliberately naive 8-direction ray-walk reference for legal-move generation
    /// and flip computation, sharing no code with [`Board::get_some_moves`]/[`Board::get_flip`],
    /// used only to cross-check them below across many random self-play games.
    mod reference {
        const DIRS: [(i32, i32); 8] = [
            (-1, -1),
            (-1, 0),
            (-1, 1),
            (0, -1),
            (0, 1),
            (1, -1),
            (1, 0),
            (1, 1),
        ];

        fn on_board(row: i32, col: i32) -> bool {
            (0..8).contains(&row) && (0..8).contains(&col)
        }

        pub fn flip(player: u64, opponent: u64, x: u32) -> u64 {
            let (row0, col0) = (x as i32 / 8, x as i32 % 8);
            let mut flipped = 0u64;
            for (dr, dc) in DIRS {
                let (mut row, mut col) = (row0 + dr, col0 + dc);
                let mut run = 0u64;
                while on_board(row, col) && opponent & (1u64 << (row * 8 + col)) != 0 {
                    run |= 1u64 << (row * 8 + col);
                    row += dr;
                    col += dc;
                }
                if on_board(row, col) && player & (1u64 << (row * 8 + col)) != 0 {
                    flipped |= run;
                }
            }
            flipped
        }

        pub fn moves(player: u64, opponent: u64) -> u64 {
            let mut moves = 0u64;
            for x in 0..64 {
                if (player | opponent) & (1u64 << x) == 0 && flip(player, opponent, x) != 0 {
                    moves |= 1u64 << x;
                }
            }
            moves
        }
    }

    /// Minimal xorshift64 PRNG so this test has no dependency on a `rand` crate — determinism
    /// isn't required here, just cheap, dependency-free pseudo-randomness for move selection.
    fn xorshift64(state: &mut u64) -> u64 {
        *state ^= *state << 13;
        *state ^= *state >> 7;
        *state ^= *state << 17;
        *state
    }

    #[test]
    fn matches_independent_reference_across_random_self_play_games() {
        let mut rng = 0x2545_f491_4f6c_dd1d_u64;
        for _game in 0..200 {
            let mut board = START;
            let mut passes_in_a_row = 0;
            for _ply in 0..120 {
                let expected_moves = reference::moves(board.player, board.opponent);
                assert_eq!(board.get_moves(), expected_moves, "board {board:?}");

                if expected_moves == 0 {
                    passes_in_a_row += 1;
                    if passes_in_a_row == 2 {
                        break;
                    }
                    board = board.pass();
                    continue;
                }
                passes_in_a_row = 0;

                let candidates: Vec<u32> = (0..64)
                    .filter(|&x| expected_moves & (1u64 << x) != 0)
                    .collect();
                let x = candidates[(xorshift64(&mut rng) as usize) % candidates.len()];
                assert_eq!(
                    board.get_flip(x),
                    reference::flip(board.player, board.opponent, x),
                    "board {board:?} x {x}"
                );
                board = board.play(x);
            }
        }
    }

    const FORCED_PASS_PLIES: [usize; 2] = [55, 57];
    const GAME_OVER_PLY: usize = 62;

    /// Parses a `%016x%016x-{b,w}` fixture entry (Go's `Board.String()` format) into a
    /// mover-relative [`Board`]: `player` is the side named by the turn suffix.
    fn parse_fixture(s: &str) -> Board {
        let (discs, turn) = s.split_at(32);
        let black = u64::from_str_radix(&discs[0..16], 16).unwrap();
        let white = u64::from_str_radix(&discs[16..32], 16).unwrap();
        match turn {
            "-b" => Board {
                player: black,
                opponent: white,
            },
            "-w" => Board {
                player: white,
                opponent: black,
            },
            _ => panic!("unexpected turn suffix: {turn}"),
        }
    }

    #[test]
    fn replays_a_real_forced_pass_game_bit_exactly() {
        let boards: Vec<Board> = FORCED_PASS_BOARDS
            .iter()
            .map(|s| parse_fixture(s))
            .collect();
        assert_eq!(boards.len(), GAME_OVER_PLY + 1);

        for ply in 0..GAME_OVER_PLY {
            let before = boards[ply];
            let after = boards[ply + 1];

            if FORCED_PASS_PLIES.contains(&ply) {
                assert!(
                    !before.can_move(),
                    "ply {ply} expected to be a forced pass, but a move exists"
                );
                assert_eq!(
                    before.pass(),
                    after,
                    "ply {ply} pass didn't reproduce the recorded next board"
                );
                continue;
            }

            assert!(
                before.can_move(),
                "ply {ply} expected a real move, but the side to move has none"
            );
            let legal = before.get_moves();
            let matches: Vec<u32> = (0..64)
                .filter(|&x| legal & (1u64 << x) != 0 && before.play(x) == after)
                .collect();
            assert_eq!(
                matches.len(),
                1,
                "ply {ply}: expected exactly one legal move reproducing the recorded next board, found {matches:?}"
            );
        }

        let final_board = boards[GAME_OVER_PLY];
        assert!(
            !final_board.can_move() && !final_board.pass().can_move(),
            "final board should be game over"
        );
    }
}
