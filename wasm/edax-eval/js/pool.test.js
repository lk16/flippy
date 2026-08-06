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

// Tests for EdaxEvalWorkerPool's scheduling: which queued search a worker starts next, and when.
// Uses a fake worker (options.createWorker) rather than real Web Workers -- none of this is about
// wasm, only about the order the pool hands work out, which is what keeps a score on screen for
// every move while deeper searches run (see static/board.js's LOCAL_EVAL_LEVELS).
//
// Needs no built wasm module, unlike edax-eval.test.js. Standalone, same shape as that file.

const assert = require('assert');
const { EdaxEvalWorkerPool, CANCELLED_MESSAGE } = require('./edax-eval');

// Board bits are irrelevant to scheduling; every task uses the starting position.
const PLAYER = 0x0000000810000000n;
const OPPONENT = 0x0000001008000000n;

// FakeWorker records the messages the pool posts and lets a test complete searches by hand, so
// "which task did the pool start, and when" is fully observable and fully deterministic.
class FakeWorker {
    constructor() {
        this.onmessage = null;
        this.onerror = null;
        this.started = []; // evaluate messages received, in order
    }

    postMessage(msg) {
        if (msg.type === 'load') {
            // Asynchronous, like a real worker's ready message: the pool is still inside its
            // constructor at this point.
            queueMicrotask(() => this.onmessage({ data: { type: 'ready' } }));
            return;
        }
        this.started.push(msg);
    }

    // running returns the evaluate message this worker has not yet answered, or null when idle.
    get running() {
        return this.started.length > this.answered ? this.started[this.started.length - 1] : null;
    }

    get answered() {
        return this._answered || 0;
    }

    // finish completes this worker's running search with `score`.
    finish(score = 0) {
        const msg = this.running;
        assert.ok(msg, 'finish() called on an idle worker');
        this._answered = this.answered + 1;
        this.onmessage({ data: { type: 'result', id: msg.id, score } });
    }

    // fail completes this worker's running search with an error instead.
    fail(error = 'boom') {
        const msg = this.running;
        assert.ok(msg, 'fail() called on an idle worker');
        this._answered = this.answered + 1;
        this.onmessage({ data: { type: 'error', id: msg.id, error } });
    }
}

// newPool builds a pool of FakeWorkers and returns it alongside them.
function newPool(numWorkers, options = {}) {
    const workers = [];
    const pool = new EdaxEvalWorkerPool('worker.js', 'x.wasm', 'w.bin.gz', numWorkers, {
        ...options,
        createWorker: () => {
            const w = new FakeWorker();
            workers.push(w);
            return w;
        },
    });
    return { pool, workers };
}

// levelsStarted lists the levels a worker has been given, in order.
function levelsStarted(worker) {
    return worker.started.map((m) => m.level);
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

    await test('runs at most one search per worker, starting the next when one finishes', async () => {
        const { pool, workers } = newPool(2);
        await pool.ready();

        for (let i = 0; i < 5; i++) pool.evaluate(PLAYER, OPPONENT, 10);
        assert.strictEqual(workers[0].started.length, 1, 'worker 0 got exactly one search');
        assert.strictEqual(workers[1].started.length, 1, 'worker 1 got exactly one search');

        workers[0].finish();
        assert.strictEqual(workers[0].started.length, 2, 'the freed worker picks up the next queued search');
        assert.strictEqual(workers[1].started.length, 1, 'the still-busy worker is not given more');
    });

    await test('starts the lowest-priority-number task first, equal priorities in arrival order', async () => {
        const { pool, workers } = newPool(1);
        await pool.ready();

        pool.evaluate(PLAYER, OPPONENT, 10, { priority: 10 }); // starts immediately: queue was empty
        pool.evaluate(PLAYER, OPPONENT, 8, { priority: 8 });
        pool.evaluate(PLAYER, OPPONENT, 4, { priority: 4 });
        pool.evaluate(PLAYER, OPPONENT, 5, { priority: 4 }); // same priority, queued later

        workers[0].finish();
        workers[0].finish();
        workers[0].finish();
        assert.deepStrictEqual(levelsStarted(workers[0]), [10, 4, 5, 8]);
    });

    await test('a task queued later still beats one queued earlier at a worse priority', async () => {
        // The point of the pool owning its backlog: a shallow search for a move that just appeared
        // on screen must not wait behind refinements queued before it.
        const { pool, workers } = newPool(1);
        await pool.ready();

        pool.evaluate(PLAYER, OPPONENT, 10, { priority: 10 });
        pool.evaluate(PLAYER, OPPONENT, 8, { priority: 8 });
        pool.evaluate(PLAYER, OPPONENT, 4, { priority: 4 });

        workers[0].finish();
        assert.deepStrictEqual(levelsStarted(workers[0]), [10, 4], 'level 4 jumped the level-8 search');
    });

    await test('the fast lane worker takes shallow searches only, idling rather than going deep', async () => {
        const { pool, workers } = newPool(2, { fastLaneMaxLevel: 4 });
        await pool.ready();

        pool.evaluate(PLAYER, OPPONENT, 10);
        pool.evaluate(PLAYER, OPPONENT, 10);
        assert.deepStrictEqual(levelsStarted(workers[0]), [], 'fast lane stays free for shallow work');
        assert.deepStrictEqual(levelsStarted(workers[1]), [10], 'deep work runs on the other worker');

        pool.evaluate(PLAYER, OPPONENT, 4);
        assert.deepStrictEqual(
            levelsStarted(workers[0]),
            [4],
            'a shallow search starts at once, without waiting for the running deep search',
        );
    });

    await test('a single-worker pool has no fast lane, so deep searches still run', async () => {
        const { pool, workers } = newPool(1, { fastLaneMaxLevel: 4 });
        await pool.ready();

        pool.evaluate(PLAYER, OPPONENT, 10);
        assert.deepStrictEqual(levelsStarted(workers[0]), [10]);
    });

    await test('cancelQueued drops queued tasks by tag and leaves running ones alone', async () => {
        const { pool, workers } = newPool(1);
        await pool.ready();

        const running = pool.evaluate(PLAYER, OPPONENT, 10, { tag: 'stale' });
        const queuedStale = pool.evaluate(PLAYER, OPPONENT, 8, { tag: 'stale' });
        const queuedFresh = pool.evaluate(PLAYER, OPPONENT, 6, { tag: 'fresh' });

        pool.cancelQueued((tag) => tag === 'stale');
        await assert.rejects(() => queuedStale, new RegExp(CANCELLED_MESSAGE));

        workers[0].finish(7);
        assert.strictEqual(await running, 7, 'the search already running was not cancelled');
        assert.deepStrictEqual(levelsStarted(workers[0]), [10, 6], 'the cancelled task never started');

        workers[0].finish(3);
        assert.strictEqual(await queuedFresh, 3, 'a task with another tag is untouched');
    });

    await test('a failed search rejects its caller and frees the worker for the next task', async () => {
        const { pool, workers } = newPool(1);
        await pool.ready();

        const failing = pool.evaluate(PLAYER, OPPONENT, 10);
        pool.evaluate(PLAYER, OPPONENT, 4);

        workers[0].fail('level 61 is not supported');
        await assert.rejects(() => failing, /level 61 is not supported/);
        assert.deepStrictEqual(levelsStarted(workers[0]), [10, 4]);
    });

    if (failures > 0) {
        console.log(`\n${failures} test(s) failed`);
        process.exit(1);
    }
    console.log('\nall tests passed');
}

main();
