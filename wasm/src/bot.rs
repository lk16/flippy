use crate::{
    position::Position,
    wasm::{console_log, current_time},
};

const CORNER_MASK: u64 = 0x8100000000000081;

/// Bot that evaluates Othello positions
pub struct Bot {
    start_time: f64,
    nodes: u64,
}

impl Bot {
    pub fn new() -> Self {
        Self {
            start_time: current_time(),
            nodes: 0,
        }
    }

    fn heuristic(&self, pos: &Position) -> i32 {
        let moves = pos.get_moves().count_ones() as i32;
        let opponent_moves = pos.get_opponent_moves().count_ones() as i32;

        if moves == 0 && opponent_moves == 0 {
            return pos.final_score();
        }

        let move_diff = moves - opponent_moves;

        let player_corners = (pos.player() & CORNER_MASK).count_ones() as i32;
        let opponent_corners = (pos.opponent() & CORNER_MASK).count_ones() as i32;
        let corner_diff = player_corners - opponent_corners;

        (3 * corner_diff) + move_diff
    }

    pub fn alpha_beta(&mut self, pos: &Position, depth: i32, alpha: i32, beta: i32) -> i32 {
        self.nodes += 1;

        if depth == 0 {
            return self.heuristic(pos);
        }

        let move_indices = pos.iter_move_indices();

        if move_indices.is_empty() {
            let passed = pos.pass();

            if !passed.has_moves() {
                return pos.final_score();
            }

            return -self.alpha_beta(&passed, depth, -beta, -alpha);
        }

        let mut alpha = alpha;

        for index in move_indices {
            let child = pos.do_move(index);
            let score = -self.alpha_beta(&child, depth - 1, -beta, -alpha);
            alpha = alpha.max(score);
            if alpha >= beta {
                break;
            }
        }

        alpha
    }

    pub fn alpha_beta_exact(&mut self, pos: &Position, alpha: i32, beta: i32) -> i32 {
        self.nodes += 1;

        let move_indices = pos.iter_move_indices();

        if move_indices.is_empty() {
            let passed = pos.pass();

            if !passed.has_moves() {
                return pos.final_score();
            }

            return -self.alpha_beta_exact(&passed, -beta, -alpha);
        }

        let mut alpha = alpha;

        for index in move_indices {
            let child = pos.do_move(index);
            let score = -self.alpha_beta_exact(&child, -beta, -alpha);
            alpha = alpha.max(score);
            if alpha >= beta {
                break;
            }
        }

        alpha
    }

    pub fn print_stats(&self) {
        let elapsed_time = current_time() - self.start_time;

        let nodes_per_second = if elapsed_time < 0.000001 {
            0
        } else {
            (self.nodes as f64 / elapsed_time) as i64
        };

        console_log(&format!(
            "Evaluated {} nodes in {:.4}s ({} nodes/s)",
            self.nodes, elapsed_time, nodes_per_second
        ));
    }
}
