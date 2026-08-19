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

// Guards the *checked-in* wasm artifact (dist/edax_eval.wasm) -- the file internal/web actually
// serves -- against going stale relative to src/. Checks, with the real shipped weights:
//   1. dist/ agrees score-for-score with a freshly built module (behavior, not bytes: a toolchain
//      upgrade legitimately changes the bytes).
//   2. a shallow (level-4) evaluation is actually fast, catching a lost level -> depth mapping.
// Requires the release wasm to have been built first:
//   cargo build --manifest-path wasm/edax-eval/Cargo.toml --target wasm32-unknown-unknown --lib --release
// To fix a failure: rerun that build and copy target/wasm32-unknown-unknown/release/edax_eval.wasm
// over dist/edax_eval.wasm.

const fs = require('fs');
const path = require('path');
const zlib = require('zlib');
const assert = require('assert');
const { EdaxEval } = require('./edax-eval');
const { OthelloPosition } = require('../../../static/board.js');

const DIST_WASM = path.join(__dirname, '..', 'dist', 'edax_eval.wasm');
const FRESH_WASM = path.join(__dirname, '..', 'target', 'wasm32-unknown-unknown', 'release', 'edax_eval.wasm');
const WEIGHTS = path.join(__dirname, '..', 'dist', 'weights.bin.gz');

// Levels to compare. Deliberately shallow (level 10 costs a few hundred ms per board); level 0
// is the raw static evaluation, so a weights/feature regression shows up too.
const LEVELS = [0, 2, 4, 6];

// Fixed corpus: deterministic pseudo-random legal games replayed to a given disc count, same
// technique as bench/bench.js, so the positions depend only on static/board.js.
const CORPUS = [
    { seed: 1, discCount: 8 },
    { seed: 3, discCount: 20 },
    { seed: 5, discCount: 32 },
    { seed: 7, discCount: 44 },
];

// LEVEL4_BUDGET_MS: a level-4 search measures ~0.3ms per board; a stale artifact ignoring the
// level takes 200-2500ms. Two orders of magnitude of headroom keeps slow CI runners from flaking.
const LEVEL4_BUDGET_MS = 25;

function playToDiscCount(seed, discCount) {
    let board = new OthelloPosition();
    let state = seed;
    const rand = () => {
        state = (state * 1103515245 + 12345) & 0x7fffffff;
        return state;
    };

    for (let moves = 0; moves < 300; moves++) {
        if (board.countDiscs('black') + board.countDiscs('white') === discCount) return board;

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
    let failures = 0;
    const test = async (name, fn) => {
        try {
            await fn();
            console.log(`ok - ${name}`);
        } catch (err) {
            failures++;
            console.log(`not ok - ${name}`);
            console.log(`  ${err.stack || err}`);
        }
    };

    if (!fs.existsSync(FRESH_WASM)) {
        console.log(
            `SKIP: ${FRESH_WASM} not found -- run ` +
                '`cargo build --manifest-path wasm/edax-eval/Cargo.toml --target wasm32-unknown-unknown --lib --release` first'
        );
        return;
    }

    const weights = new Uint8Array(zlib.gunzipSync(fs.readFileSync(WEIGHTS)));
    const dist = await EdaxEval.instantiate(fs.readFileSync(DIST_WASM), weights);
    const fresh = await EdaxEval.instantiate(fs.readFileSync(FRESH_WASM), weights);

    const positions = CORPUS.map(({ seed, discCount }) => {
        const board = playToDiscCount(seed, discCount);
        assert.ok(board, `seed=${seed} discCount=${discCount}: position not reached`);
        return { seed, discCount, board };
    });

    await test('dist/edax_eval.wasm scores match a fresh build of src/', async () => {
        for (const { seed, discCount, board } of positions) {
            for (const level of LEVELS) {
                const distScore = dist.evaluate(board.playerBits, board.opponentBits, level);
                const freshScore = fresh.evaluate(board.playerBits, board.opponentBits, level);
                assert.strictEqual(
                    distScore,
                    freshScore,
                    `seed=${seed} discCount=${discCount} level=${level}: dist/ says ${distScore}, ` +
                        `a fresh build says ${freshScore} -- dist/edax_eval.wasm is stale, rebuild and copy it over`
                );
            }
        }
    });

    await test(`dist/edax_eval.wasm evaluates level 4 in under ${LEVEL4_BUDGET_MS}ms per board`, async () => {
        for (const { seed, discCount, board } of positions) {
            const start = process.hrtime.bigint();
            dist.evaluate(board.playerBits, board.opponentBits, 4);
            const elapsedMs = Number(process.hrtime.bigint() - start) / 1e6;
            assert.ok(
                elapsedMs < LEVEL4_BUDGET_MS,
                `seed=${seed} discCount=${discCount}: level 4 took ${elapsedMs.toFixed(1)}ms -- ` +
                    'shallow levels are supposed to be near-instant (is the level -> depth mapping still applied?)'
            );
        }
    });

    if (failures > 0) {
        console.log(`\n${failures} test(s) failed`);
        process.exit(1);
    }
    console.log('\nall tests passed');
}

main();
