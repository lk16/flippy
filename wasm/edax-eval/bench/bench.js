#!/usr/bin/env node
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

// Fixed-corpus wall-clock benchmark for search::solve, run through the actual compiled wasm
// module (the deployment target) rather than a native build, since wasm-specific codegen is
// exactly what some candidate optimizations (e.g. popcount/ctz lowering) affect.
//
// Usage: build the release wasm first, then run this script:
//   cargo build --manifest-path wasm/edax-eval/Cargo.toml --target wasm32-unknown-unknown --lib --release
//   node wasm/edax-eval/bench/bench.js
//
// The corpus is a fixed set of (seed, discCount) pairs, each replayed via static/board.js's
// OthelloBoard through a deterministic pseudo-random legal game (same technique used to sanity
// check the browser wiring end-to-end -- see the WASM-triggering fix this benchmark followed).
// Fully deterministic and independent of anything in wasm/edax-eval's Rust source, so the same
// positions are evaluated before and after a candidate change -- only search::solve's speed on
// them can differ, not which positions get benchmarked. Mixes both level-10 regimes: several
// midgame (n_empties > 20, scored by search_eval_0 at depth-10 leaves) and several endgame
// (n_empties <= 20, exact-solved to the end of the game) positions. Total wall-clock across the
// whole corpus is the metric to compare run-to-run -- individual positions vary by 10-50x in cost
// (no transposition table means branching-factor luck matters a lot at the harder depths), so only
// the fixed-corpus total is meaningful, not any single position's time.

const fs = require('fs');
const path = require('path');
const zlib = require('zlib');
const { EdaxEval } = require('../js/edax-eval.js');
const { OthelloBoard } = require('../../../static/board.js');

const WASM_PATH = path.join(__dirname, '../target/wasm32-unknown-unknown/release/edax_eval.wasm');
const WEIGHTS_PATH = path.join(__dirname, '../dist/weights.bin.gz');

// Midgame corpus: two seeds, sampled at four disc counts each, all in the n_empties > 20
// (depth-10 fixed-depth) regime.
const MIDGAME_CASES = [3, 5].flatMap((seed) => [36, 38, 40, 42].map((discCount) => ({ seed, discCount, level: 10 })));

// Level-12 midgame corpus: only n_empties > 24 positions (discCount < 40), so
// depth_and_selectivity(12, n_empties) → dep=12, sel=0 (true midgame, ProbCut fires).
// n_empties <= 24 → exact-solve at depth 22-24 with no ProbCut → too slow for this benchmark.
// Used to verify TT speedup for level 11+ (Task 16): compare with-TT vs. baseline (Task 15).
const MIDGAME12_CASES = [3, 5].flatMap((seed) => [36, 38].map((discCount) => ({ seed, discCount, level: 12 })));

// Endgame corpus (level 10): seven seeds at n_empties 8-14 (light endgame) plus four seeds at n_empties 16
// (hard endgame — still within the exact-solve regime but deep enough that move-ordering and
// search-tree improvements show up clearly). All individually timed against the release wasm to
// stay under ~400ms each. n_empties 17+ are excluded: too position-dependent without a
// transposition table (some take seconds, others minutes). n_empties 15 is similarly excluded:
// several tested seeds took 500ms-1s against the baseline build.
const ENDGAME_CASES = [
    // n_empties 8, 10, 12, 14: light endgame, seven seeds each
    ...[1, 2, 3, 4, 5, 7, 8].flatMap((seed) =>
        [50, 52, 54, 56].map((discCount) => ({ seed, discCount, level: 10 })),
    ),
    // n_empties 16: hard endgame, four seeds (seeds 2-5 stay under ~140ms baseline each)
    ...[2, 3, 4, 5].map((seed) => ({ seed, discCount: 48, level: 10 })),
];

function popcount(bits) {
    let n = bits;
    let count = 0n;
    while (n > 0n) {
        count += n & 1n;
        n >>= 1n;
    }
    return Number(count);
}

function discCountOf(board) {
    return popcount(board.playerBits) + popcount(board.opponentBits);
}

// playToDiscCount replays a deterministic pseudo-random legal game (a simple LCG seeded by
// `seed`) until a board with exactly `discCount` discs is reached, or returns null if the game
// ends first. Deterministic and independent of any Rust/wasm behavior -- only static/board.js's
// OthelloBoard (a separate JS port) drives it.
function playToDiscCount(seed, discCount) {
    let board = new OthelloBoard();
    let state = seed;
    const rand = () => {
        state = (state * 1103515245 + 12345) & 0x7fffffff;
        return state;
    };

    for (let moves = 0; moves < 300; moves++) {
        if (discCountOf(board) === discCount) return board.toString();

        const legalMoves = [...Array(64).keys()].filter((i) => board.isValidMove(i));
        if (legalMoves.length === 0) {
            const passed = board.clone();
            passed.passMove();
            if (![...Array(64).keys()].some((i) => passed.isValidMove(i))) return null; // game over
            board = passed;
            continue;
        }
        board = board.doMove(legalMoves[rand() % legalMoves.length]);
    }
    return null;
}

async function main() {
    const wasmBytes = fs.readFileSync(WASM_PATH);
    const weightsBytes = zlib.gunzipSync(fs.readFileSync(WEIGHTS_PATH));
    const evaluator = await EdaxEval.instantiate(wasmBytes, weightsBytes);

    const cases = [...MIDGAME_CASES, ...MIDGAME12_CASES, ...ENDGAME_CASES].map(({ seed, discCount, level }) => {
        const boardStr = playToDiscCount(seed, discCount);
        if (!boardStr) throw new Error(`seed=${seed} discCount=${discCount}: position not reached`);
        return { seed, discCount, level, board: OthelloBoard.fromString(boardStr) };
    });

    let totalNanos = 0n;
    for (const { seed, discCount, level, board } of cases) {
        const start = process.hrtime.bigint();
        const score = evaluator.evaluate(board.playerBits, board.opponentBits, level);
        const elapsed = process.hrtime.bigint() - start;
        totalNanos += elapsed;
        console.log(
            `seed=${seed} discCount=${discCount} n_empties=${64 - discCount} level=${level}: ` +
                `score=${score} time=${(Number(elapsed) / 1e6).toFixed(1)}ms`,
        );
    }

    console.log(`\nTOTAL: ${(Number(totalNanos) / 1e6).toFixed(1)}ms across ${cases.length} positions`);
}

main().catch((error) => {
    console.error('FAILED:', error);
    process.exit(1);
});
