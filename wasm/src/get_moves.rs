//! This file was copied over from swap, which is a Rust adaption of Edax. https://github.com/lk16/swap

/// Get bitset of valid moves.
///
/// Like get_moves() in Edax.
pub fn get_moves(player: u64, opponent: u64) -> u64 {
    let mask = opponent & 0x7E7E7E7E7E7E7E7E;
    let mut moves = 0;

    // Define directions and their masks
    let directions = [
        (1, mask),     // horizontal
        (7, mask),     // diagonal up-right/down-left
        (9, mask),     // diagonal up-left/down-right
        (8, opponent), // vertical
    ];

    for &(dir, dir_mask) in &directions {
        let mut flip_l = dir_mask & (player << dir);
        flip_l |= dir_mask & (flip_l << dir);
        let mask_l = dir_mask & (dir_mask << dir);
        flip_l |= mask_l & (flip_l << (2 * dir));
        flip_l |= mask_l & (flip_l << (2 * dir));

        let mut flip_r = dir_mask & (player >> dir);
        flip_r |= dir_mask & (flip_r >> dir);
        let mask_r = dir_mask & (dir_mask >> dir);
        flip_r |= mask_r & (flip_r >> (2 * dir));
        flip_r |= mask_r & (flip_r >> (2 * dir));

        moves |= (flip_l << dir) | (flip_r >> dir);
    }

    moves &= !(player | opponent);
    moves
}

#[cfg(test)]
pub mod tests {
    use super::*;
    use crate::bitset::tests::print_bitset;
    use crate::position::Position;

    pub const XOT_POSITIONS: Vec<Position> = vec![];

    /// Shift a bitset in a given direction.
    fn shift(bitset: u64, dir: i32) -> u64 {
        match dir {
            -9 => (bitset & 0xfefefefefefefefe) << 7,
            -8 => bitset << 8,
            -7 => (bitset & 0x7f7f7f7f7f7f7f7f) << 9,
            -1 => (bitset & 0xfefefefefefefefe) >> 1,
            1 => (bitset & 0x7f7f7f7f7f7f7f7f) << 1,
            7 => (bitset & 0x7f7f7f7f7f7f7f7f) >> 7,
            8 => bitset >> 8,
            9 => (bitset & 0xfefefefefefefefe) >> 9,
            _ => panic!("Invalid direction"),
        }
    }

    pub fn get_moves_simple(player: u64, opponent: u64) -> u64 {
        let empty = !(player | opponent);
        let mut moves = 0;

        // Define direction offsets
        const DIRECTIONS: [i32; 8] = [-9, -8, -7, -1, 1, 7, 8, 9];

        for dir in DIRECTIONS {
            let mut candidates = shift(player, dir) & opponent;

            while candidates != 0 {
                moves |= empty & shift(candidates, dir);
                candidates = shift(candidates, dir) & opponent;
            }
        }
        moves
    }

    pub fn move_test_cases() -> Vec<Position> {
        const DIRECTIONS: [(i32, i32); 8] = [
            (-1, -1),
            (-1, 0),
            (-1, 1),
            (0, -1),
            (0, 1),
            (1, -1),
            (1, 0),
            (1, 1),
        ];

        let mut test_cases = Vec::new();

        for i in 0..64 {
            for (dx, dy) in DIRECTIONS {
                for distance in 1..=6 {
                    let x = i % 8;
                    let y = i / 8;

                    let move_x = x + (distance + 1) * dx;
                    let move_y = y + (distance + 1) * dy;

                    if !(0..8).contains(&move_x) || !(0..8).contains(&move_y) {
                        continue;
                    }

                    let player = 1 << i;

                    let mut opponent = 0;
                    for d in 1..=distance {
                        let index = (y + d * dy) * 8 + (x + d * dx);
                        opponent |= 1 << (index as usize);
                    }

                    test_cases.push(Position::new_from_bitboards(player, opponent));
                }
            }
        }

        test_cases.extend(XOT_POSITIONS.iter());

        test_cases
    }

    #[test]
    fn test_get_moves_simple() {
        let positions = move_test_cases();

        for position in positions {
            let player = position.player();
            let opponent = position.opponent();

            let simple = get_moves_simple(player, opponent);
            let regular = get_moves(player, opponent);

            println!("Position:");
            println!("{position}");

            println!("Regular:");
            print_bitset(regular);

            println!("Simple:");
            print_bitset(simple);

            assert_eq!(simple, regular);
        }
    }
}
