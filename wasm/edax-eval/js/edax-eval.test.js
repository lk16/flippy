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

// Node-runnable smoke test for edax-eval.js, exercising the real wasm module end to end (not a
// mock): builds a wasm32 binary via cargo, generates a structurally-valid (but not real-weights)
// gzip blob the same shape DecompressionStream('gzip') would hand instantiate() after a real
// fetch, and checks the alloc/init_weights/evaluate protocol round-trips and reports errors
// correctly. Deliberately separate from static/test/'s framework (run.js/framework.js): that
// suite tests static/'s browser JS, which this isn't wired into yet (TASKS.md Task 10).
//
// Requires: `cargo build --manifest-path wasm/edax-eval/Cargo.toml --target wasm32-unknown-unknown
// --lib --release` already run (this script does not build it, to keep it fast to iterate on and
// to match how a real page would just fetch a prebuilt .wasm).
//
// Weight *correctness* (bit-exactness against real Edax) is Task 8's job
// (tests/differential.rs) -- this only tests the JS loading/plumbing layer.

const fs = require('fs');
const path = require('path');
const zlib = require('zlib');
const assert = require('assert');
const { EdaxEval } = require('./edax-eval');

const WASM_PATH = path.join(
    __dirname,
    '..',
    'target',
    'wasm32-unknown-unknown',
    'release',
    'edax_eval.wasm'
);

// Must match weights_transform::N_W * weights_transform::N_PLIES * 2 (src/weights_transform.rs).
const N_W = 114364;
const N_PLIES = 52;
const WEIGHTS_BLOB_LEN = N_W * N_PLIES * 2;

// Othello starting position, mover-relative (black to move): matches board::START in Rust.
const START_PLAYER = 0x0000000810000000n;
const START_OPPONENT = 0x0000001008000000n;

function syntheticGzippedWeightsBlob() {
    // Structurally valid (right length) but not real trained weights -- see this file's header
    // comment on why that's fine here.
    const raw = Buffer.alloc(WEIGHTS_BLOB_LEN);
    for (let i = 0; i < raw.length; i++) raw[i] = (i * 7 + 3) % 256;
    return zlib.gzipSync(raw);
}

async function decompressGzip(gzipped) {
    // Exercises the same DecompressionStream('gzip') API load() uses, not Node's zlib module, so
    // this test covers the actual browser-facing code path.
    const stream = new Blob([gzipped]).stream().pipeThrough(new DecompressionStream('gzip'));
    return new Uint8Array(await new Response(stream).arrayBuffer());
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

    if (!fs.existsSync(WASM_PATH)) {
        console.log(
            `SKIP: ${WASM_PATH} not found -- run ` +
                '`cargo build --manifest-path wasm/edax-eval/Cargo.toml --target wasm32-unknown-unknown --lib --release` first'
        );
        return;
    }
    const wasmBytes = fs.readFileSync(WASM_PATH);
    const weightsBytes = await decompressGzip(syntheticGzippedWeightsBlob());
    assert.strictEqual(weightsBytes.length, WEIGHTS_BLOB_LEN, 'decompressed blob has the expected length');

    await test('instantiate() loads weights and evaluate() returns an in-range score', async () => {
        const edax = await EdaxEval.instantiate(wasmBytes, weightsBytes);
        const score = edax.evaluate(START_PLAYER, START_OPPONENT, 10);
        assert.ok(Number.isInteger(score), `score ${score} should be an integer`);
        assert.ok(score >= -64 && score <= 64, `score ${score} should be in [-64, 64]`);
    });

    await test('evaluate() rejects a level above 60', async () => {
        const edax = await EdaxEval.instantiate(wasmBytes, weightsBytes);
        assert.throws(() => edax.evaluate(START_PLAYER, START_OPPONENT, 61), /level 61 is not supported/);
    });

    await test('evaluate() accepts every supported level', async () => {
        // Levels each map to their own depth/selectivity (search::depth_and_selectivity); the
        // scores themselves are meaningless here (synthetic weights), so this only pins the ABI:
        // no level in 0..=60 hits a sentinel. That levels genuinely differ in *result* is checked
        // against the real weights, in dist-freshness.test.js.
        const edax = await EdaxEval.instantiate(wasmBytes, weightsBytes);
        for (const level of [0, 1, 4, 10]) {
            const score = edax.evaluate(START_PLAYER, START_OPPONENT, level);
            assert.ok(score >= -64 && score <= 64, `level ${level}: score ${score} should be in [-64, 64]`);
        }
        // Levels 11+ are checked on a 5-empties board instead: from the starting position they
        // mean "solve the whole game exactly", which does not finish in a test's lifetime. Same
        // position and reasoning as search.rs's accepts_levels_zero_through_sixty.
        for (const level of [11, 60]) {
            const score = edax.evaluate(0x007ee4d2aadcbe7cn, 0x7e011b2d55234180n, level);
            assert.ok(score >= -64 && score <= 64, `level ${level}: score ${score} should be in [-64, 64]`);
        }
    });

    await test('instantiate() rejects a wrong-length weights blob', async () => {
        await assert.rejects(
            () => EdaxEval.instantiate(wasmBytes, weightsBytes.slice(0, 100)),
            /wrong length/
        );
    });

    if (failures > 0) {
        console.log(`\n${failures} test(s) failed`);
        process.exit(1);
    }
    console.log('\nall tests passed');
}

main();
