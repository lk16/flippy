mod bitset;
mod bot;
mod get_flipped;
mod get_moves;
mod position;
mod wasm;

use crate::position::Position;

use bot::Bot;
use wasm::console_error;
use wasm_bindgen::prelude::*;

/// Evaluates a position at given depth and returns the score for the player to move
#[wasm_bindgen]
pub fn evaluate_position(position_string: &str, depth: i32) -> i32 {
    let position = match Position::from_string(position_string) {
        Ok(pos) => pos,
        Err(e) => {
            console_error(&format!("Error parsing position: {e}"));
            return 0;
        }
    };

    let mut bot = Bot::new();
    let score = bot.alpha_beta(&position, depth, -64, 64);
    bot.print_stats();
    score
}

/// Evaluates a position and returns the exact score for the player to move
#[wasm_bindgen]
pub fn evaluate_position_exact(position_string: &str) -> i32 {
    let position = match Position::from_string(position_string) {
        Ok(pos) => pos,
        Err(e) => {
            console_error(&format!("Error parsing position: {}", e));
            return 0;
        }
    };

    let mut bot = Bot::new();
    let score = bot.alpha_beta_exact(&position, -64, 64);
    bot.print_stats();
    score
}
