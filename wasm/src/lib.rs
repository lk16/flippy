#![allow(unused)] // TODO remove

use std::time::Instant;
use wasm_bindgen::prelude::*;

const CORNER_MASK: u64 = 0x8100000000000081;
const PASS_MOVE: i32 = -1;

/// Represents an Othello position using bitboards
#[derive(Clone, Copy)]
pub struct Position {
    player_discs: u64,
    opponent_discs: u64,
}

impl Position {
    /// Creates a new position with the given bitboards
    pub fn new(player_discs: u64, opponent_discs: u64) -> Self {
        Self {
            player_discs,
            opponent_discs,
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

        let player_discs = u64::from_str_radix(&s[0..16], 16)
            .map_err(|e| format!("invalid player discs hex: {}", e))?;
        let opponent_discs = u64::from_str_radix(&s[16..32], 16)
            .map_err(|e| format!("invalid opponent discs hex: {}", e))?;

        // Check that the bitboards don't overlap
        if player_discs & opponent_discs != 0 {
            return Err("player and opponent discs cannot overlap".to_string());
        }

        Ok(Self::new(player_discs, opponent_discs))
    }

    /// Returns the bitboard of valid moves for the current player
    pub fn moves(&self) -> u64 {
        todo!() // TODO
    }

    /// Returns true if there are any valid moves
    pub fn has_moves(&self) -> bool {
        todo!() // TODO
    }

    /// Returns the bitboard of the current player's discs
    pub fn player_discs(&self) -> u64 {
        self.player_discs
    }

    /// Returns the bitboard of the opponent's discs
    pub fn opponent_discs(&self) -> u64 {
        self.opponent_discs
    }

    /// Returns the final score of the game
    pub fn final_score(&self) -> i32 {
        let player_count = self.player_discs.count_ones() as i32;
        let opponent_count = self.opponent_discs.count_ones() as i32;

        if player_count > opponent_count {
            64 - (2 * opponent_count)
        } else if opponent_count > player_count {
            -64 + (2 * player_count)
        } else {
            0
        }
    }

    /// Makes a move and returns the new position
    pub fn do_move(&self, mv: i32) -> Position {
        if mv == PASS_MOVE {
            // For pass move, just swap the players
            Position::new(self.opponent_discs, self.player_discs)
        } else {
            todo!() // TODO
        }
    }

    /// Returns all possible child positions
    pub fn get_children(&self) -> Vec<Position> {
        todo!() // TODO
    }
}

/// Bot that evaluates Othello positions
pub struct Bot {
    start_time: Instant,
    nodes: u64,
}

impl Bot {
    pub fn new() -> Self {
        Self {
            start_time: Instant::now(),
            nodes: 0,
        }
    }

    fn heuristic(&self, pos: &Position) -> i32 {
        let passed = pos.do_move(PASS_MOVE);

        let moves = pos.moves().count_ones() as i32;
        let opponent_moves = passed.moves().count_ones() as i32;

        if moves == 0 && opponent_moves == 0 {
            return pos.final_score();
        }

        let move_diff = moves - opponent_moves;

        let player_corners = (pos.player_discs() & CORNER_MASK).count_ones() as i32;
        let opponent_corners = (pos.opponent_discs() & CORNER_MASK).count_ones() as i32;
        let corner_diff = player_corners - opponent_corners;

        (3 * corner_diff) + move_diff
    }

    fn alpha_beta(&mut self, pos: &Position, depth: i32, alpha: i32, beta: i32) -> i32 {
        self.nodes += 1;

        if depth == 0 {
            return self.heuristic(pos);
        }

        let children = pos.get_children();

        if children.is_empty() {
            let passed = pos.do_move(PASS_MOVE);

            if !passed.has_moves() {
                return -pos.final_score();
            }

            return -self.alpha_beta(&passed, depth, -beta, -alpha);
        }

        let mut alpha = alpha;

        for child in children {
            let score = -self.alpha_beta(&child, depth - 1, -beta, -alpha);

            if score >= beta {
                return beta;
            }

            if score > alpha {
                alpha = score;
            }
        }

        alpha
    }

    pub fn print_stats(&self) {
        let elapsed_seconds = self.start_time.elapsed().as_secs_f64();

        let nodes_per_second = if elapsed_seconds > 0.000001 {
            (self.nodes as f64 / elapsed_seconds) as i64
        } else {
            0
        };

        let _msg = &format!(
            "Evaluated {} nodes in {:.4}s ({} nodes/s)",
            self.nodes, elapsed_seconds, nodes_per_second
        );

        // TODO actually log the message
    }
}

/// Evaluates a position and returns the score for the player to move
#[wasm_bindgen]
pub fn evaluate_position(position_string: &str, depth: i32) -> i32 {
    let _position = match Position::from_string(position_string) {
        Ok(pos) => pos,
        Err(e) => {
            web_sys::console::error_1(&format!("Error parsing position: {}", e).into());
            return 0;
        }
    };

    // TODO use this:
    // let mut bot = Bot::new();
    // let score = bot.alpha_beta(&position, depth, -64, 64);
    // bot.print_stats();
    // score

    69
}
