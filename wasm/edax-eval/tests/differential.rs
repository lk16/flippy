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

//! Differential correctness harness (`TASKS.md` Tasks 8 + 11): runs a small board corpus through
//! both this crate's `search::solve` and the real `lEdax-x64` binary at levels 10 and below,
//! asserting the scores match exactly. Gated on `EDAX_PATH` (and `EVAL_DAT_PATH`/`EDAX_HOST_DIR`
//! for the real weights), skipped rather than failed when unset, mirroring
//! `internal/edax/process_test.go`'s pattern so this needs no separate CI exclusion (CI has
//! neither set, per `docs/project.md`).
//!
//! The problem-line format and output-parsing protocol are reimplemented here rather than shared
//! with `internal/edax` (`problem.go`/`parser.go`/`process.go`) since that package is Go and this
//! crate is Rust; the encoding itself is copied deliberately closely (real board colors and turn,
//! not mover-relative -- see `[[project_edax-color-turn-encoding]]`'s lesson, referenced in
//! TASKS.md's Background, that `-solve` problem lines need real colors) and was manually checked
//! against one real `lEdax-x64` invocation before writing this file's parser (see this file's git
//! history / commit message for that transcript).
//!
//! **Corpus size note (no silent cap):** this crate's search has no transposition table or move
//! ordering yet (TASKS.md's "Explicitly deferred" section), so runtime for a given `n_empties` is
//! wildly position-dependent, not just a function of depth -- measured directly (a throwaway
//! calibration harness, not committed) against many random positions at fixed `n_empties`:
//!
//! | `n_empties` | fastest seen | slowest seen |
//! |---|---|---|
//! | 13 | 8.9ms | 1.2s |
//! | 14 | 50.7ms | 5.9s |
//! | 15 | 134ms | 8.4s |
//! | 16 | 445ms | 43s |
//! | 17 | 928ms | 103s |
//!
//! i.e. two positions at the *same* `n_empties` can differ in cost by 100-1000x depending on how
//! much the (missing) move ordering/pruning would have helped. `n_empties` 18-20 (the top of the
//! exact-solve regime -- recall level 10 solves exactly whenever `n_empties <= 20`, see TASKS.md
//! Background) were not explored past 17: the trend above makes it plausible individual positions
//! there take minutes to tens of minutes, and finding a fast one isn't guaranteed by trying a few
//! seeds (17 itself took two multi-minute misses before a sub-second one turned up). This is a
//! known, deliberate gap, not a silent one -- revisit once a transposition table lands. This test
//! runs under `cargo test --release` (`test.sh` passes `--release` for exactly this reason); a bare
//! `cargo test` without it will be substantially slower across the whole corpus below.
//!
//! Every position below was individually timed before being added (again via the throwaway
//! harness) and picked to stay under ~1.2s in `--release`, so the corpus is deliberately larger
//! than just "one of each regime": several exact-solve samples spanning `n_empties` 8-17 (covering
//! the previously-untested 13-17 gap), and three midgame samples (`n_empties` 28, 32, and ~39,
//! exercising `search_eval_0` at different depths-to-leaf) instead of just one.

use std::io::Write;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};

use edax_eval::board::{Board, START};
use edax_eval::search::solve;
use edax_eval::weights::{unpack, EvalWeight};
use edax_eval::weights_transform::{N_PLIES, N_W};

fn edax_path() -> Option<PathBuf> {
    std::env::var("EDAX_PATH")
        .ok()
        .filter(|p| !p.is_empty())
        .map(PathBuf::from)
}

fn eval_dat_path() -> Option<PathBuf> {
    if let Ok(p) = std::env::var("EVAL_DAT_PATH") {
        return Some(PathBuf::from(p));
    }
    std::env::var("EDAX_HOST_DIR")
        .ok()
        .map(|d| PathBuf::from(d).join("data").join("eval.dat"))
}

fn load_real_weights(path: &Path) -> Vec<EvalWeight> {
    let file =
        std::fs::read(path).unwrap_or_else(|e| panic!("failed to read {}: {e}", path.display()));

    const HEADER_LEN: usize = 28;
    const FIRST_PLY: usize = 2;
    const EVAL_N_PLY: usize = 54;
    let block_bytes = N_W * 2;

    let mut raw = vec![0i16; N_W * N_PLIES];
    for ply in FIRST_PLY..EVAL_N_PLY {
        let block_start = HEADER_LEN + ply * block_bytes;
        let block = &file[block_start..block_start + block_bytes];
        let out_ply = ply - FIRST_PLY;
        for i in 0..N_W {
            raw[out_ply * N_W + i] = i16::from_le_bytes([block[i * 2], block[i * 2 + 1]]);
        }
    }
    unpack(&raw, N_W, N_PLIES)
}

const TABLE_BORDER: &str =
    "------+-----+--------------+-------------+----------+---------------------";

/// Mirrors `internal/edax/parser.go`'s `parseFinalEvaluation`: the final result is the last
/// parseable depth/score data row before the *second* table-border line.
fn parse_final_score(output: &str) -> Option<i32> {
    let mut last = None;
    let mut border_seen = 0;
    for line in output.lines() {
        if line.contains(TABLE_BORDER) {
            border_seen += 1;
            if border_seen == 2 {
                return last;
            }
            continue;
        }
        if let Some(score) = parse_result_line(line) {
            last = Some(score);
        }
    }
    last
}

/// Mirrors `parseResultLine`/`parseDepthConfidence`: a data row's first field is `depth` or
/// `depth@confidence%`, its second is the score (or `<`/`>`-prefixed for a non-exact bound, which
/// means the search isn't done at that row -- skip it). Header/art/banner lines all fail the
/// "first field parses as an integer" check, so no dedicated line-shape filtering is needed beyond
/// that (verified against a real `lEdax-x64 -verbose 3` transcript, not assumed).
fn parse_result_line(line: &str) -> Option<i32> {
    let fields: Vec<&str> = line.split_whitespace().collect();
    if fields.len() < 2 {
        return None;
    }
    let depth_str = fields[0].split('@').next().unwrap();
    if depth_str.parse::<i32>().is_err() {
        return None;
    }
    let score_field = fields[1];
    if score_field.starts_with('<') || score_field.starts_with('>') {
        return None;
    }
    score_field.parse::<i32>().ok()
}

/// Runs one real-board position through the real edax binary at `level`, returning its exact final
/// score from `mover_is_black`'s point of view. Mirrors `internal/edax`'s problem-line encoding
/// (`problem.go`) and subprocess invocation (`process.go`'s `buildArgs`/`ensureStarted`).
fn edax_solve(edax_path: &Path, black: u64, white: u64, mover_is_black: bool, level: u32) -> i32 {
    let mut squares = [b'-'; 64];
    for i in 0..64u32 {
        if black & (1u64 << i) != 0 {
            squares[i as usize] = b'X';
        } else if white & (1u64 << i) != 0 {
            squares[i as usize] = b'O';
        }
    }
    let mut problem = String::from_utf8(squares.to_vec()).unwrap();
    problem.push(' ');
    problem.push(if mover_is_black { 'X' } else { 'O' });
    problem.push_str(";\n");

    // process.go: cmd.Dir = filepath.Join(filepath.Dir(p.path), ".."), so options.c's default
    // relative "data/eval.dat" resolves from the edax-reversi checkout root, not the bin/ dir.
    let edax_dir = edax_path
        .parent()
        .and_then(Path::parent)
        .unwrap_or_else(|| Path::new("."));

    let level_str = level.to_string();
    let mut child = Command::new(edax_path)
        .current_dir(edax_dir)
        .args([
            "-solve",
            "/dev/stdin",
            "-level",
            &level_str,
            "-verbose",
            "3",
        ])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .unwrap_or_else(|e| panic!("failed to start {}: {e}", edax_path.display()));

    child
        .stdin
        .take()
        .expect("stdin was piped")
        .write_all(problem.as_bytes())
        .expect("failed to write problem line to edax");

    let output = child.wait_with_output().expect("failed to wait for edax");
    let stdout = String::from_utf8_lossy(&output.stdout);
    parse_final_score(&stdout).unwrap_or_else(|| {
        panic!("no parseable score in edax output:\n{stdout}\nproblem line: {problem}")
    })
}

/// One corpus entry: a real-color board plus which side is to move.
struct Position {
    black: u64,
    white: u64,
    mover_is_black: bool,
}

fn to_position(board: &Board, mover_is_black: bool) -> Position {
    let (black, white) = if mover_is_black {
        (board.player, board.opponent)
    } else {
        (board.opponent, board.player)
    };
    Position {
        black,
        white,
        mover_is_black,
    }
}

/// Plays random legal moves from the start position (own move generator, per TASKS.md decision
/// #6 -- board/move-gen is a separately-verified port, not shared code) until `n_empties` first
/// reaches `target_empties` or below, tracking which physical color is to move. `player`/
/// `opponent` swap on every ply (move *or* pass) alike, so the physical color a mover-relative
/// `Board.player` represents alternates every ply regardless of which caused the swap.
fn play_until(target_empties: u32, seed: u64) -> Position {
    let mut rng = seed;
    let mut xorshift = || {
        rng ^= rng << 13;
        rng ^= rng >> 7;
        rng ^= rng << 17;
        rng
    };

    let mut board = START;
    let mut mover_is_black = true;
    loop {
        let n_empties = 64 - (board.player | board.opponent).count_ones();
        if n_empties <= target_empties {
            break;
        }
        let moves = board.get_moves();
        if moves == 0 {
            board = board.pass();
            mover_is_black = !mover_is_black;
            continue;
        }
        let candidates: Vec<u32> = (0..64).filter(|&x| moves & (1u64 << x) != 0).collect();
        board = board.play(candidates[(xorshift() as usize) % candidates.len()]);
        mover_is_black = !mover_is_black;
    }
    assert_ne!(
        board.get_moves(),
        0,
        "corpus position must have a legal move for the side to move"
    );
    to_position(&board, mover_is_black)
}

#[test]
fn matches_real_edax_at_level_10() {
    let Some(edax) = edax_path() else {
        eprintln!("EDAX_PATH not set; skipping test requiring the real edax binary");
        return;
    };
    let Some(eval_dat) = eval_dat_path() else {
        eprintln!("EVAL_DAT_PATH/EDAX_HOST_DIR not set; skipping test requiring real eval.dat");
        return;
    };
    let weights = load_real_weights(&eval_dat);

    let mut corpus = Vec::new();
    // Midgame regime (n_empties > 20): exercises the depth-10/search_eval_0 path. Three samples at
    // different depths-to-leaf, each individually timed to stay under ~1.2s in --release (see this
    // file's doc comment) -- (seed, target n_empties) pairs picked by that calibration, not
    // arbitrary.
    for (seed, target) in [(32u64, 28), (32, 32), (1, 40)] {
        corpus.push(play_until(target, seed));
    }
    // Exact-solve regime: board_solve/pass-to-game-over, search_eval_0 never reached. Spans
    // n_empties 8-17 (the 13-17 gap noted in this file's doc comment is filled in here), every
    // (seed, target) pair individually timed to stay under ~1.2s in --release.
    for (seed, target) in [
        (4u64, 12),
        (5, 10),
        (6, 8),
        (13, 13),
        (14, 14),
        (13, 15),
        (13, 16),
        (22, 17),
    ] {
        corpus.push(play_until(target, seed));
    }

    for pos in corpus {
        let board = if pos.mover_is_black {
            Board {
                player: pos.black,
                opponent: pos.white,
            }
        } else {
            Board {
                player: pos.white,
                opponent: pos.black,
            }
        };
        let n_empties = 64 - (board.player | board.opponent).count_ones();

        let ours = solve(&board, 10, &weights).expect("level 10 is always supported");
        let real = edax_solve(&edax, pos.black, pos.white, pos.mover_is_black, 10);

        assert_eq!(
            ours, real,
            "score mismatch at n_empties={n_empties}: ours={ours}, real edax={real}, black={:#018x}, white={:#018x}, mover_is_black={}",
            pos.black, pos.white, pos.mover_is_black
        );
    }
}

/// Task 11: verify levels below 10 match real edax at those levels. Uses a subset of the
/// existing corpus positions (already timed at level 10; lower levels are strictly faster).
/// Covers one midgame-regime position (n_empties=28, n_empties > 2*level for both levels tested)
/// and one exact-solve-regime position (n_empties=8, n_empties <= 2*level for both) at each level.
#[test]
fn matches_real_edax_at_levels_below_ten() {
    let Some(edax) = edax_path() else {
        eprintln!("EDAX_PATH not set; skipping test requiring the real edax binary");
        return;
    };
    let Some(eval_dat) = eval_dat_path() else {
        eprintln!("EVAL_DAT_PATH/EDAX_HOST_DIR not set; skipping test requiring real eval.dat");
        return;
    };
    let weights = load_real_weights(&eval_dat);

    // (level, seed, target_n_empties) — same seeds as matches_real_edax_at_level_10's corpus.
    let cases: &[(u32, u64, u32)] = &[
        (5, 32, 28), // level 5 midgame: n_empties=28 > 2*5=10 → depth=5
        (5, 6, 8),   // level 5 exact-solve: n_empties=8 <= 2*5=10 → depth=8
        (8, 32, 28), // level 8 midgame: n_empties=28 > 2*8=16 → depth=8
        (8, 6, 8),   // level 8 exact-solve: n_empties=8 <= 2*8=16 → depth=8
    ];

    for &(level, seed, target) in cases {
        let pos = play_until(target, seed);
        let board = if pos.mover_is_black {
            Board {
                player: pos.black,
                opponent: pos.white,
            }
        } else {
            Board {
                player: pos.white,
                opponent: pos.black,
            }
        };
        let n_empties = 64 - (board.player | board.opponent).count_ones();

        let ours = solve(&board, level, &weights).expect("levels 1-10 are always supported");
        let real = edax_solve(&edax, pos.black, pos.white, pos.mover_is_black, level);

        assert_eq!(
            ours, real,
            "score mismatch at level={level}, n_empties={n_empties}: ours={ours}, real edax={real}, black={:#018x}, white={:#018x}, mover_is_black={}",
            pos.black, pos.white, pos.mover_is_black
        );
    }
}
