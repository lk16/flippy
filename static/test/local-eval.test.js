// Tests for queueLocalEvaluations' incremental-depth refinement: each off-book board is
// evaluated through LOCAL_EVAL_LEVELS (4, 6, 8, 10) in order, one independent chain per board,
// so the UI shows a shallow score quickly and sharpens it as deeper wasm searches complete.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame, LOCAL_EVAL_LEVELS } = require('./harness');
const { FORCED_PASS_BOARDS } = require('./fixtures');

function flush() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

// mockWorkerPool records every evaluate() call and lets the test resolve them one at a time,
// mirroring EdaxEvalWorkerPool's real signature (player, opponent, level) -> Promise<score>.
function mockWorkerPool() {
  const calls = [];
  return {
    calls,
    evaluate(player, opponent, level) {
      return new Promise((resolve) => {
        calls.push({ player, opponent, level, resolved: false, resolve });
      });
    },
  };
}

test('LOCAL_EVAL_LEVELS is the documented incremental depth sequence', () => {
  assert.deepEqual(LOCAL_EVAL_LEVELS, [4, 6, 8, 10]);
});

test('queueLocalEvaluations: refines a board through LOCAL_EVAL_LEVELS in order', async () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const boardStr = game.pgnAllChildStrings[0];
  game.queueLocalEvaluations([boardStr]);

  for (const level of LOCAL_EVAL_LEVELS) {
    await flush();
    const call = pool.calls.find((c) => c.level === level && !c.resolved);
    assert.ok(call, `expected a pending evaluate() call at level ${level}`);
    call.resolved = true;
    call.resolve(level); // score == level, so assertions can check both at once
    await flush();
    const e = game.evaluations.get(boardStr);
    assert.equal(e.level, level);
    assert.equal(e.score, level);
    assert.equal(e.source, 'wasm');
  }

  assert.equal(pool.calls.length, LOCAL_EVAL_LEVELS.length, 'exactly one evaluate() call per level, no more');
  assert.ok(!game._pendingLocalEvals.has(boardStr), 'no longer pending once the chain completes');
});

test('queueLocalEvaluations: stops refining once a server-sourced evaluation supersedes the board', async () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const boardStr = game.pgnAllChildStrings[0];
  game.queueLocalEvaluations([boardStr]);
  await flush();

  assert.equal(pool.calls.length, 1);
  assert.equal(pool.calls[0].level, 4);

  // A real (server) evaluation arrives while the level-4 worker call is still in flight.
  game.evaluations.set(boardStr, { board: boardStr, score: 3, source: 'edax', level: 10 });

  pool.calls[0].resolve(999);
  await flush();
  await flush();

  assert.equal(game.evaluations.get(boardStr).source, 'edax', 'server result is not overwritten by the stale wasm chain');
  assert.equal(pool.calls.length, 1, 'chain stops instead of continuing on to deeper levels');
  assert.ok(!game._pendingLocalEvals.has(boardStr));
});

test('queueLocalEvaluations: different boards dispatch independently (no waiting on each other)', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const [a, b] = game.pgnAllChildStrings;
  game.queueLocalEvaluations([a, b]);

  // Both boards' first (level-4) calls are dispatched up front, without waiting for either to
  // resolve -- this is what lets EdaxEvalWorkerPool's round-robin send them to different workers.
  assert.equal(pool.calls.length, 2);
  assert.deepEqual(pool.calls.map((c) => c.level), [4, 4]);
});

test('queueLocalEvaluations: skips boards that already have an evaluation or are already pending', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const [a, b] = game.pgnAllChildStrings;
  game.evaluations.set(a, { board: a, score: 1, source: 'edax', level: 10 });
  game._pendingLocalEvals.add(b);

  game.queueLocalEvaluations([a, b]);

  assert.equal(pool.calls.length, 0, 'neither an already-evaluated nor an already-pending board dispatches a worker call');
});
