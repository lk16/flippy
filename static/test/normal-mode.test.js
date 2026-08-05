// Tests for normal (non-PGN) play's evaluation wiring: requestMissingEvaluations/
// requestGrandchildrenEvaluations must ask the server to actually analyze on-book positions it
// has never seen (analyze_request), not just look up whatever it already has saved
// (evaluation_request) -- otherwise a position outside the pre-explored book never gets evaluated,
// since nothing else ever enqueues it. Off-book positions still go to the local wasm chain and
// need no server request or polling at all.
//
// The children of the current board additionally *always* go to the local wasm chain while the
// server hasn't answered for them, on-book or not: that is what puts a score under every legal
// move immediately instead of after the server's (seconds-to-minutes) analysis.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildNormalGame, OthelloBoard } = require('./harness');

function recordingWsClient() {
  const sent = [];
  return {
    sent,
    requestEvaluations(boards) { if (boards.length) sent.push({ event: 'evaluation_request', boards }); },
    sendEvent(event, boards, level) { if (boards.length) sent.push({ event, boards, level }); },
  };
}

// recordingWorkerPool mimics EdaxEvalWorkerPool's evaluate/cancelQueued signatures, recording
// every dispatched (player, opponent, level, options) without ever resolving, so a test can see
// exactly which boards the local wasm chain was asked for and at what priority. bitsOf() below
// maps a board string to the same key.
function recordingWorkerPool() {
  const calls = [];
  const cancelled = [];
  return {
    calls,
    cancelled,
    keys: () => new Set(calls.map((c) => `${c.player}:${c.opponent}`)),
    evaluate(player, opponent, level, options = {}) {
      calls.push({ player, opponent, level, ...options });
      return new Promise(() => {});
    },
    cancelQueued(shouldDrop) {
      cancelled.push(shouldDrop);
    },
  };
}

function bitsOf(boardStr) {
  const b = OthelloBoard.fromString(boardStr);
  return `${b.playerBits}:${b.opponentBits}`;
}

function childStrings(board) {
  return [...new Set(board.getChildren().map((c) => c.normalize().toString()))];
}

test('requestMissingEvaluations: sends both evaluation_request and analyze_request for an unexplored on-book position', () => {
  const game = buildNormalGame();
  game.wsClient = recordingWsClient();

  game.requestMissingEvaluations(game.board);

  const evReq = game.wsClient.sent.find((m) => m.event === 'evaluation_request');
  const anReq = game.wsClient.sent.find((m) => m.event === 'analyze_request');
  assert.ok(evReq, 'evaluation_request was sent');
  assert.ok(anReq, 'analyze_request was sent -- normal mode must ask the server to compute, not just look up');
  assert.equal(anReq.level, game.levelConfig.priorityLevel);
});

test('requestGrandchildrenEvaluations: also sends analyze_request for prefetched grandchildren', () => {
  const game = buildNormalGame();
  game.wsClient = recordingWsClient();

  game.requestGrandchildrenEvaluations(game.board);

  const anReq = game.wsClient.sent.find((m) => m.event === 'analyze_request');
  assert.ok(anReq, 'analyze_request was sent for grandchildren');
});

test('requestMissingEvaluations: off-book children skip server requests entirely (wasm handles them)', () => {
  const game = buildNormalGame();
  game.levelConfig.maxSavableDiscs = 4; // starting position's children (5 discs) are now off-book
  game.wsClient = recordingWsClient();
  game.edaxWorkerPool = recordingWorkerPool();

  game.requestMissingEvaluations(game.board);

  assert.equal(game.wsClient.sent.length, 0, 'no evaluation_request or analyze_request for off-book boards');
});

test('requestMissingEvaluations: on-book children go to the local wasm chain too, at level 4 first', () => {
  const game = buildNormalGame();
  game.wsClient = recordingWsClient();
  const pool = recordingWorkerPool();
  game.edaxWorkerPool = pool;

  game.requestMissingEvaluations(game.board);

  const expected = new Set(childStrings(game.board).map(bitsOf));
  assert.deepEqual(pool.keys(), expected, 'every child is queued locally, not just off-book ones');
  assert.ok(pool.calls.every((c) => c.level === 4), 'each chain starts at the shallowest level');
  assert.ok(
    game.wsClient.sent.some((m) => m.event === 'analyze_request'),
    'the server is still asked for the real evaluation',
  );
});

test('requestGrandchildrenEvaluations: on-book grandchildren are not queued locally', () => {
  const game = buildNormalGame();
  game.wsClient = recordingWsClient();
  const pool = recordingWorkerPool();
  game.edaxWorkerPool = pool;

  game.requestGrandchildrenEvaluations(game.board);

  assert.equal(pool.calls.length, 0, 'invisible on-book grandchildren must not delay the visible children');
});

test('requestMissingEvaluations: a wasm score does not count as the server having answered', () => {
  const game = buildNormalGame();
  game.wsClient = recordingWsClient();
  for (const key of childStrings(game.board)) {
    game.evaluations.set(key, { board: key, score: 0, source: 'wasm', level: 4 });
  }

  game.requestMissingEvaluations(game.board);

  const anReq = game.wsClient.sent.find((m) => m.event === 'analyze_request');
  assert.ok(anReq, 'the server is still asked to compute a board that only has a local wasm score');
  assert.deepEqual(new Set(anReq.boards), new Set(childStrings(game.board)));
  assert.ok(game.hasUnresolvedEvaluations(game.board), 'polling keeps waiting for the server result');
});

test('requestMissingEvaluations: does not re-request boards that already have an evaluation', () => {
  const game = buildNormalGame();
  game.wsClient = recordingWsClient();
  for (const child of game.board.getChildren()) {
    const key = child.normalize().toString();
    game.evaluations.set(key, { board: key, score: 0, source: 'edax', level: 10 });
  }

  game.requestMissingEvaluations(game.board);

  assert.equal(game.wsClient.sent.length, 0, 'nothing requested once every child already has an evaluation');
});

test('hasUnresolvedEvaluations: true when an on-book child/grandchild lacks an evaluation, false once resolved', () => {
  const game = buildNormalGame();
  assert.ok(game.hasUnresolvedEvaluations(game.board), 'fresh board has no evaluations yet');

  const seen = new Set();
  for (const child of game.board.getChildren()) {
    seen.add(child.normalize().toString());
    for (const grandchild of child.getChildren()) seen.add(grandchild.normalize().toString());
  }
  for (const key of seen) {
    game.evaluations.set(key, { board: key, score: 0, source: 'edax', level: 10 });
  }
  assert.equal(game.hasUnresolvedEvaluations(game.board), false, 'resolved once every child/grandchild has an evaluation');
});

test('hasUnresolvedEvaluations: off-book boards never count as unresolved (no server-side wait to poll for)', () => {
  const game = buildNormalGame();
  game.levelConfig.maxSavableDiscs = 4; // starting position's children are now off-book
  assert.equal(game.hasUnresolvedEvaluations(game.board), false);
});

test('updateValidMoves: starts eval polling while on-book evaluations are missing, stops once resolved', () => {
  const game = buildNormalGame();
  game.wsClient = recordingWsClient();

  game.updateValidMoves(game.board);
  assert.ok(game.evalPollTimer !== null, 'polling started for an unresolved on-book position');

  game.stopEvalPolling();
  for (const child of game.board.getChildren()) {
    const key = child.normalize().toString();
    game.evaluations.set(key, { board: key, score: 0, source: 'edax', level: 10 });
    for (const grandchild of child.getChildren()) {
      const gkey = grandchild.normalize().toString();
      game.evaluations.set(gkey, { board: gkey, score: 0, source: 'edax', level: 10 });
    }
  }
  game.updateValidMoves(game.board);
  assert.equal(game.evalPollTimer, null, 'polling does not (re)start once everything is resolved');
});

test('setPGNState: stops any running normal-mode eval poll', () => {
  const game = buildNormalGame();
  game.wsClient = recordingWsClient();
  game.updateValidMoves(game.board);
  assert.ok(game.evalPollTimer !== null);

  // setPGNState touches DOM elements the harness stubs out; only the polling side effect matters here.
  game.setPGNState('input');
  assert.equal(game.evalPollTimer, null, 'entering the PGN section stops normal-mode polling');
});

test('a board with valid moves always has children to evaluate (sanity check for the fixtures above)', () => {
  const board = new OthelloBoard();
  assert.ok(board.getChildren().length > 0);
});
