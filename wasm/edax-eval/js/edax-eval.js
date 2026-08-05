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

// Minimal JS wrapper (TASKS.md Task 10) for wasm/edax-eval's raw wasm ABI (src/wasm_api.rs). Plain
// script, no build step and no wasm-bindgen (docs/project.md: this repo has no frontend build
// step) -- follows static/board.js's convention: a plain <script>-loadable file under the browser,
// module.exports under Node for testing (see edax-eval.test.js).
//
// Not wired into any page yet (TASKS.md Task 10 is explicit that's a separate follow-up) -- this
// only exposes the class.

// Sentinel return values from evaluate()/init_weights() (src/wasm_api.rs). i32::MIN and
// i32::MIN + 1 in two's complement 32-bit -- kept as plain numbers, not derived, so a change to
// the Rust side is a visible diff here too, not something that silently drifts.
const ERR_WEIGHTS_NOT_INITIALIZED = -2147483648; // i32::MIN
const ERR_UNSUPPORTED_LEVEL = -2147483647; // i32::MIN + 1
const ERR_WRONG_LENGTH = 1;
const ERR_ALREADY_INITIALIZED = 2;

// EdaxEval wraps one instantiated wasm module plus its loaded weights. Construct via
// EdaxEval.instantiate (from raw bytes) or EdaxEval.load (fetches from URLs) -- not directly via
// `new`, since loading is inherently async.
class EdaxEval {
    // wasmInstance: a WebAssembly.Instance whose exports include memory/alloc/init_weights/evaluate
    // (src/wasm_api.rs). Not called directly; use instantiate()/load().
    constructor(wasmInstance) {
        this.instance = wasmInstance;
    }

    // Instantiates the wasm module from raw bytes and loads weightsBytes (the *decompressed*
    // transpose+delta+byte-plane-encoded blob -- see wasm_api.rs's init_weights doc comment; the
    // caller is responsible for decompression, e.g. via DecompressionStream('gzip'), see load()
    // below). Returns a ready-to-use EdaxEval.
    static async instantiate(wasmBytes, weightsBytes) {
        const { instance } = await WebAssembly.instantiate(wasmBytes, {});
        const exports = instance.exports;

        const ptr = exports.alloc(weightsBytes.length);
        new Uint8Array(exports.memory.buffer, ptr, weightsBytes.length).set(weightsBytes);

        const initResult = exports.init_weights(ptr, weightsBytes.length);
        if (initResult === ERR_WRONG_LENGTH) {
            throw new Error(
                `edax-eval: weights blob is the wrong length (${weightsBytes.length} bytes) -- ` +
                    'did decompression run, and is this the blob wasm/edax-eval/src/bin/extract_weights.rs produced?'
            );
        }
        if (initResult === ERR_ALREADY_INITIALIZED) {
            throw new Error('edax-eval: this wasm instance already has weights loaded');
        }
        if (initResult !== 0) {
            throw new Error(`edax-eval: init_weights failed with unrecognized code ${initResult}`);
        }

        return new EdaxEval(instance);
    }

    // Fetches wasmUrl and weightsUrl, gzip-decompresses the latter via the browser's native
    // DecompressionStream, and instantiates. This is the convenience path a real page would use;
    // instantiate() above (raw bytes in, no fetch) is what makes this module testable under Node
    // without a real server -- see edax-eval.test.js.
    static async load(wasmUrl, weightsUrl) {
        const [wasmBytes, weightsResponse] = await Promise.all([
            fetch(wasmUrl).then((r) => r.arrayBuffer()),
            fetch(weightsUrl),
        ]);
        const decompressedStream = weightsResponse.body.pipeThrough(new DecompressionStream('gzip'));
        const weightsBytes = new Uint8Array(await new Response(decompressedStream).arrayBuffer());
        return EdaxEval.instantiate(wasmBytes, weightsBytes);
    }

    // Evaluates a board at Edax level (0-60; each level maps to its own search depth and
    // selectivity, so a shallow level really is cheaper -- see search::solve and
    // depth_and_selectivity. Levels above 60 throw rather than silently running as 60).
    // player/opponent are Edax's mover-relative bitboards (crate::board::Board -- player
    // is the side to move), as BigInt (wasm i64 params require BigInt from JS, not Number).
    // Converting from a black/white/turn representation to mover-relative is the caller's job, not
    // this wrapper's (same reasoning as [[project_edax-color-turn-encoding]] elsewhere in this repo:
    // getting that conversion right matters and shouldn't be silently implicit).
    evaluate(player, opponent, level) {
        const score = this.instance.exports.evaluate(player, opponent, level);
        if (score === ERR_WEIGHTS_NOT_INITIALIZED) {
            throw new Error('edax-eval: evaluate() called before weights finished loading');
        }
        if (score === ERR_UNSUPPORTED_LEVEL) {
            throw new Error(`edax-eval: level ${level} is not supported (levels 0-60 only)`);
        }
        return score;
    }
}

// CANCELLED_MESSAGE is the rejection message cancelQueued() gives a dropped task, so callers can
// tell "this position stopped mattering" apart from a real evaluation failure.
const CANCELLED_MESSAGE = 'edax-eval: evaluation cancelled';

// EdaxEvalWorkerPool manages N Web Worker instances each running one wasm/edax-eval instance.
// Workers evaluate boards concurrently -- one active evaluation per worker -- so the main thread
// never blocks on WASM. Each worker loads the wasm module and weights independently.
//
// The pool owns its backlog rather than posting every request straight to a worker: a task waits
// in this._queue until a worker is free, so *ordering stays changeable up to the moment a search
// starts*. That is what makes evaluate()'s `priority` meaningful -- a cheap shallow search queued
// later still runs before an expensive deep one queued earlier (a level-4 search costs well under
// a millisecond, a level-10 one tens to hundreds), which is how the caller keeps a score on screen
// for every move while deeper refinement continues in the background.
//
// Worker script (edax-eval-worker.js) must live beside this file, served at the same URL prefix.
class EdaxEvalWorkerPool {
    // options.fastLaneMaxLevel: when set (and numWorkers > 1), worker 0 becomes a fast lane that
    // accepts only tasks at or below this level, staying *idle* rather than picking up a deeper
    // one. Priority alone can't bound how long a shallow search waits -- every worker may already
    // be mid-search on a level-10 board, and a running wasm search can't be interrupted -- so
    // without a reserved lane "show a number immediately" would still mean "in up to a second".
    // The cost is one worker's worth of deep-search throughput.
    //
    // options.createWorker: injection point for tests (no Worker API in Node); defaults to the
    // real thing.
    constructor(workerUrl, wasmUrl, weightsUrl, numWorkers, options = {}) {
        const { fastLaneMaxLevel = null, createWorker = (url) => new Worker(url) } = options;
        this._numWorkers = numWorkers;
        this._fastLaneMaxLevel = numWorkers > 1 ? fastLaneMaxLevel : null;
        this._nextId = 0;
        this._queue = []; // accepted but not yet started, in arrival order
        this._busy = new Array(numWorkers).fill(false);
        this._pending = new Map(); // id -> { resolve, reject, workerIndex }
        this._readyCount = 0;
        this._readyResolve = null;
        this._readyReject = null;
        this._readyPromise = new Promise((resolve, reject) => {
            this._readyResolve = resolve;
            this._readyReject = reject;
        });
        this._workers = Array.from({ length: numWorkers }, () => {
            const w = createWorker(workerUrl);
            w.onmessage = ({ data: msg }) => this._onMessage(msg);
            w.onerror = (err) => console.error('edax-eval worker error:', err);
            w.postMessage({ type: 'load', wasmUrl, weightsUrl });
            return w;
        });
    }

    // Resolves when all workers have loaded their wasm instance and weights.
    ready() {
        return this._readyPromise;
    }

    // Evaluates one board asynchronously. player/opponent are BigInt mover-relative bitboards.
    // Returns a Promise<number> (the score from player's POV).
    //
    // options.priority: lower numbers start first; equal priorities start in arrival order.
    // options.tag: opaque value handed back to cancelQueued()'s predicate, for dropping this
    // task later if it stops being worth running.
    evaluate(player, opponent, level, options = {}) {
        const { priority = 0, tag = null } = options;
        return new Promise((resolve, reject) => {
            this._queue.push({ id: this._nextId++, player, opponent, level, priority, tag, resolve, reject });
            this._dispatch();
        });
    }

    // cancelQueued drops every queued task whose tag satisfies shouldDrop(tag), rejecting its
    // promise with CANCELLED_MESSAGE. Tasks already running are left alone -- a wasm search runs
    // to completion inside its worker and cannot be interrupted -- so they still resolve normally.
    cancelQueued(shouldDrop) {
        const kept = [];
        const dropped = [];
        for (const task of this._queue) (shouldDrop(task.tag) ? dropped : kept).push(task);
        this._queue = kept;
        for (const task of dropped) task.reject(new Error(CANCELLED_MESSAGE));
    }

    // _dispatch hands each idle worker the best task it is allowed to run, if any.
    _dispatch() {
        for (let i = 0; i < this._numWorkers && this._queue.length > 0; i++) {
            if (this._busy[i]) continue;
            const task = this._takeTaskFor(i);
            if (!task) continue;
            this._busy[i] = true;
            this._pending.set(task.id, { resolve: task.resolve, reject: task.reject, workerIndex: i });
            this._workers[i].postMessage({
                type: 'evaluate',
                id: task.id,
                player: task.player,
                opponent: task.opponent,
                level: task.level,
            });
        }
    }

    // _takeTaskFor removes and returns the task worker i should run next, or null if the queue
    // holds nothing it may run. Scanning in arrival order and replacing only on a *strictly*
    // lower priority number keeps equal priorities FIFO.
    _takeTaskFor(workerIndex) {
        const fastLaneOnly = workerIndex === 0 && this._fastLaneMaxLevel !== null;
        let best = -1;
        for (let j = 0; j < this._queue.length; j++) {
            if (fastLaneOnly && this._queue[j].level > this._fastLaneMaxLevel) continue;
            if (best === -1 || this._queue[j].priority < this._queue[best].priority) best = j;
        }
        return best === -1 ? null : this._queue.splice(best, 1)[0];
    }

    _onMessage(msg) {
        if (msg.type === 'ready') {
            this._readyCount++;
            if (this._readyCount === this._numWorkers) this._readyResolve();
            return;
        }
        if (msg.type === 'load_error') {
            this._readyReject(new Error(msg.error));
            return;
        }
        const p = this._pending.get(msg.id);
        if (!p) return;
        this._pending.delete(msg.id);
        this._busy[p.workerIndex] = false;
        if (msg.type === 'result') {
            p.resolve(msg.score);
        } else if (msg.type === 'error') {
            p.reject(new Error(msg.error));
        }
        this._dispatch();
    }
}

// In the browser (loaded via <script>) EdaxEval/EdaxEvalWorkerPool are just globals. Under Node
// (e.g. this directory's test harness) `module` exists; export them there instead, matching
// static/board.js's/static/test's own dual-mode convention. EdaxEvalWorkerPool needs no Worker
// API under Node as long as tests pass options.createWorker (see pool.test.js).
if (typeof module !== 'undefined') {
    module.exports = { EdaxEval, EdaxEvalWorkerPool, CANCELLED_MESSAGE };
}
