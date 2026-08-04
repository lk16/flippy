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

// Web Worker script: runs the edax-eval WASM module off the main thread so evaluations don't
// block the browser UI. The main thread sends two kinds of messages:
//   { type: 'load',     wasmUrl, weightsUrl }  — fetches + initializes this worker's wasm instance
//   { type: 'evaluate', id, player, opponent, level }  — evaluates one board, posts back result
//
// Evaluate messages that arrive before the load is complete are buffered and processed afterwards.
// `importScripts` is relative to this file's URL, so it resolves to the sibling edax-eval.js.

importScripts('./edax-eval.js'); // defines EdaxEval as a global

let edaxEval = null;
const buffered = []; // evaluate messages that arrived before load completed

self.onmessage = async function ({ data: msg }) {
    if (msg.type === 'load') {
        try {
            edaxEval = await EdaxEval.load(msg.wasmUrl, msg.weightsUrl);
            self.postMessage({ type: 'ready' });
            for (const m of buffered) handleEvaluate(m);
            buffered.length = 0;
        } catch (err) {
            self.postMessage({ type: 'load_error', error: String(err) });
        }
    } else if (msg.type === 'evaluate') {
        if (!edaxEval) {
            buffered.push(msg);
        } else {
            handleEvaluate(msg);
        }
    }
};

function handleEvaluate(msg) {
    try {
        const score = edaxEval.evaluate(msg.player, msg.opponent, msg.level);
        self.postMessage({ id: msg.id, type: 'result', score });
    } catch (err) {
        self.postMessage({ id: msg.id, type: 'error', error: String(err) });
    }
}
