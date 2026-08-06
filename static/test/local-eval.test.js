// Tests for queueLocalEvaluations' incremental-depth refinement: each board is evaluated through
// LOCAL_EVAL_LEVELS (4, 6, 8, 10) in order, so the UI shows a shallow score quickly and sharpens
// it as deeper wasm searches complete -- and the searches are queued so that the shallow ones all
// run first (priority), and so that leaving a position abandons its unstarted work (tag).
// EdaxEvalWorkerPool's side of that contract is tested in wasm/edax-eval/js/pool.test.js.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame, buildNormalGame, OthelloBoard, LOCAL_EVAL_LEVELS } = require('./harness');
const { FORCED_PASS_BOARDS } = require('./fixtures');

function flush() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

// mockWorkerPool records every evaluate() call and lets the test resolve them one at a time,
// mirroring EdaxEvalWorkerPool's real signature (player, opponent, level, options) ->
// Promise<score>. cancelQueued() drops every not-yet-resolved call whose tag matches, the same
// way the real pool drops tasks that haven't started yet.
function mockWorkerPool() {
  const calls = [];
  return {
    calls,
    evaluate(player, opponent, level, { priority = 0, tag = null } = {}) {
      return new Promise((resolve, reject) => {
        calls.push({ player, opponent, level, priority, tag, resolved: false, resolve, reject });
      });
    },
    cancelQueued(shouldDrop) {
      for (const call of calls) {
        if (call.resolved || !shouldDrop(call.tag)) continue;
        call.resolved = true;
        call.reject(new Error('edax-eval: evaluation cancelled'));
      }
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

  // Both boards' first (level-4) calls are queued up front, without waiting for either to
  // resolve -- this is what lets the pool run them on separate workers at the same time.
  assert.equal(pool.calls.length, 2);
  assert.deepEqual(pool.calls.map((c) => c.level), [4, 4]);
});

test('queueLocalEvaluations: priority is the search level, with an offset for off-screen prefetch', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const [a, b] = game.pgnAllChildStrings;
  game.queueLocalEvaluations([a]);
  game.queueLocalEvaluations([b], { prefetch: true });

  // Shallow-first across boards (level as priority), and nothing off screen ever precedes
  // something on screen -- LOCAL_EVAL_PREFETCH_PRIORITY is larger than any level.
  assert.deepEqual(pool.calls.map((c) => c.priority), [4, 104]);
});

test('queueLocalEvaluations: resumes a partly refined board at the next deeper level', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const boardStr = game.pgnAllChildStrings[0];
  game.evaluations.set(boardStr, { board: boardStr, score: 1, level: 6, source: 'wasm' });
  game.queueLocalEvaluations([boardStr]);

  assert.equal(pool.calls.length, 1);
  assert.equal(pool.calls[0].level, 8, 'picks up above the level already reached, not from scratch');
});

test('queueLocalEvaluations: skips a board a local chain already took to the deepest level', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const boardStr = game.pgnAllChildStrings[0];
  const deepest = LOCAL_EVAL_LEVELS[LOCAL_EVAL_LEVELS.length - 1];
  game.evaluations.set(boardStr, { board: boardStr, score: 1, level: deepest, source: 'wasm' });
  game.queueLocalEvaluations([boardStr]);

  assert.equal(pool.calls.length, 0);
});

test('queueLocalEvaluations: a late stale result never downgrades a deeper score', async () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const boardStr = game.pgnAllChildStrings[0];
  game.queueLocalEvaluations([boardStr]);
  await flush();

  // A deeper local result lands first (a search that was already running when the position
  // changed can finish after a fresher chain has taken the same board further).
  game.evaluations.set(boardStr, { board: boardStr, score: 2, level: 8, source: 'wasm' });
  pool.calls[0].resolve(99);
  await flush();

  const e = game.evaluations.get(boardStr);
  assert.equal(e.level, 8);
  assert.equal(e.score, 2);
});

test('requestMissingEvaluations: moving on abandons the previous position\'s local work', async () => {
  // One move in: the starting position's own children are all symmetric to each other, so it
  // would queue a single search -- too few to tell a cancelled queue from an empty one.
  const game = buildNormalGame(new OthelloBoard().getChildren()[0]);
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  const childCount = new Set(game.board.getChildren().map((c) => c.normalize().toString())).size;
  game.requestMissingEvaluations(game.board);
  assert.equal(pool.calls.length, childCount, 'one level-4 search per distinct child');
  const generation = game._localEvalGeneration;
  assert.ok(pool.calls.every((c) => c.tag === generation), 'every search is tagged with the position it belongs to');

  // One search has already started, so the pool can no longer drop it; the rest are still queued.
  const running = pool.calls[0];
  running.resolved = true;

  game.requestMissingEvaluations(game.board.getChildren()[0]);

  const queued = pool.calls.slice(1, childCount);
  assert.ok(queued.every((c) => c.resolved), 'the queued searches for the old position were cancelled');
  assert.notEqual(game._localEvalGeneration, generation);
  const fresh = pool.calls.slice(childCount);
  assert.ok(fresh.length > 0, 'the new position queued its own searches');
  assert.ok(fresh.every((c) => c.level === 4 && c.tag === game._localEvalGeneration));

  const beforeStaleResult = pool.calls.length;
  running.resolve(5);
  await flush();
  await flush();

  assert.equal(pool.calls.length, beforeStaleResult, 'the stale chain stops instead of queueing its next level');
  assert.ok(
    [...game.evaluations.values()].some((e) => e.source === 'wasm' && e.score === 5),
    'its result is still kept -- it is a correct evaluation, just not refined further',
  );
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
