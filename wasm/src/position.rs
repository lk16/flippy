use std::fmt::{self, Display};

use crate::{get_flipped::get_flipped, get_moves::get_moves};

/// Represents an Othello position using bitboards
#[derive(Clone, Copy)]
pub struct Position {
    player: u64,
    opponent: u64,
}

impl Display for Position {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "{}", self.ascii_art(true))
    }
}

impl Position {
    /// Creates a new position with the given bitboards
    pub fn new_from_bitboards(player: u64, opponent: u64) -> Self {
        let position = Self { player, opponent };
        position.validate();
        position
    }

    fn validate(&self) {
        #[cfg(debug_assertions)]
        if self.player & self.opponent != 0 {
            panic!("player and opponent discs cannot overlap");
        }
    }

    /// Creates a new position from a string representation
    /// The string should be a 32-character hex string, where the first 16 characters
    /// represent the player's discs and the last 16 characters represent the opponent's discs
    pub fn from_string(s: &str) -> Result<Self, String> {
        if s.len() != 32 {
            return Err(format!(
                "position string must be 32 characters long, got {}",
                s.len()
            ));
        }

        let player = u64::from_str_radix(&s[0..16], 16)
            .map_err(|e| format!("invalid player discs hex: {e}"))?;
        let opponent = u64::from_str_radix(&s[16..32], 16)
            .map_err(|e| format!("invalid opponent discs hex: {e}"))?;

        Ok(Self::new_from_bitboards(player, opponent))
    }

    /// Compute bitset of valid moves for the player to move.
    pub fn get_moves(&self) -> u64 {
        get_moves(self.player, self.opponent)
    }

    /// Compute bitset of valid moves for the opponent.
    pub fn get_opponent_moves(&self) -> u64 {
        get_moves(self.opponent, self.player)
    }

    /// Returns true if there are any valid moves
    pub fn has_moves(&self) -> bool {
        self.get_moves() != 0
    }

    /// Returns the bitboard of the current player's discs
    pub fn player(&self) -> u64 {
        self.player
    }

    /// Returns the bitboard of the opponent's discs
    pub fn opponent(&self) -> u64 {
        self.opponent
    }

    /// Returns the final score of the game
    pub fn final_score(&self) -> i32 {
        let player_count = self.player.count_ones() as i32;
        let opponent_count = self.opponent.count_ones() as i32;

        match player_count.cmp(&opponent_count) {
            std::cmp::Ordering::Greater => 64 - (2 * opponent_count),
            std::cmp::Ordering::Less => -64 + (2 * player_count),
            std::cmp::Ordering::Equal => 0,
        }
    }

    pub fn pass(&self) -> Position {
        Position::new_from_bitboards(self.opponent, self.player)
    }

    /// Makes a move and returns the new position
    pub fn do_move(&self, index: usize) -> Position {
        debug_assert!(index < 64);

        let move_bit = 1u64 << index;
        debug_assert!(self.get_moves() & move_bit != 0);

        let flipped = get_flipped(self.player, self.opponent, index);

        let player = self.opponent ^ flipped;
        let opponent = self.player | flipped | move_bit;
        Position::new_from_bitboards(player, opponent)
    }

    /// Return iterator over move indices.
    pub fn iter_move_indices(&self) -> MoveIndices {
        MoveIndices::new(self.get_moves())
    }

    /// Returns an ASCII art representation of the board.
    pub fn ascii_art(&self, black_to_move: bool) -> String {
        let player_char;
        let opponent_char;
        let black_count;
        let white_count;
        let black_moves;
        let white_moves;
        let black_move_arrow;
        let white_move_arrow;

        if black_to_move {
            player_char = "○";
            opponent_char = "●";
            black_count = self.player.count_ones();
            white_count = self.opponent.count_ones();
            black_moves = self.get_moves().count_ones();
            white_moves = self.get_opponent_moves().count_ones();
            black_move_arrow = "->";
            white_move_arrow = "  ";
        } else {
            player_char = "●";
            opponent_char = "○";
            black_count = self.opponent.count_ones();
            white_count = self.player.count_ones();
            black_moves = self.get_opponent_moves().count_ones();
            white_moves = self.get_moves().count_ones();
            black_move_arrow = "  ";
            white_move_arrow = "->";
        }

        let moves = self.get_moves();

        let mut lines = vec![];
        lines.push("+-A-B-C-D-E-F-G-H-+".to_string());
        for row in 0..8 {
            let mut line = String::new();
            line.push_str(&format!("{} ", row + 1));
            for col in 0..8 {
                let index = row * 8 + col;
                let mask = 1u64 << index;
                if self.player & mask != 0 {
                    line.push_str(&format!("{player_char} "));
                } else if self.opponent & mask != 0 {
                    line.push_str(&format!("{opponent_char} "));
                } else if moves & mask != 0 {
                    line.push_str("· ");
                } else {
                    line.push_str("  ");
                }
            }
            line.push_str(&format!("{}", row + 1));
            lines.push(line);
        }
        lines.push("+-A-B-C-D-E-F-G-H-+".to_string());

        let space = "   ";

        lines[2] +=
            &format!("{space} {black_move_arrow} ○ {black_count:2} - {black_moves:2} moves");
        lines[3] +=
            &format!("{space} {white_move_arrow} ● {white_count:2} - {white_moves:2} moves");

        lines[7] += &format!(
            "{}(0x{:016X}, 0x{:016X})",
            space, self.player, self.opponent
        );

        lines.join("\n") + "\n"
    }
}

pub struct MoveIndices {
    remaining_moves: u64,
}

impl MoveIndices {
    pub fn new(remaining_moves: u64) -> Self {
        Self { remaining_moves }
    }

    pub fn is_empty(&self) -> bool {
        self.remaining_moves == 0
    }
}

impl Iterator for MoveIndices {
    type Item = usize;

    fn next(&mut self) -> Option<Self::Item> {
        if self.remaining_moves == 0 {
            return None;
        }

        let index = self.remaining_moves.trailing_zeros() as usize;
        self.remaining_moves &= self.remaining_moves - 1;
        Some(index)
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        let size = self.remaining_moves.count_ones() as usize;
        (size, Some(size))
    }
}
