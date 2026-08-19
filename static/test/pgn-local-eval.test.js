// Tests for the local (wasm) evaluation of a whole reviewed PGN line: the score graph must cover
// every ply, not just the ones the book already holds, so every board it is drawn from gets a
// level-4 search immediately and refines from there. That work belongs to the line rather than to
// the board on screen, so stepping through plies must not abandon it -- but loading another PGN
// must. See pgnQueueLineEvaluations / _localEvalLineTag in board.js.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame, mockWorkerPool, flush, MAX_TARGET_LEVEL, OthelloBoard } = require('./harness');
const { FORCED_PASS_BOARDS } = require('./fixtures');

// resolveAll resolves every pending call recorded so far with `score`, and returns how many it
// resolved. Calls the chains queue afterwards (the next level up) are left pending.
async function resolveAll(pool, score) {
  const pending = pool.calls.filter((c) => !c.resolved);
  for (const call of pending) {
    call.resolved = true;
    call.resolve(score);
  }
  await flush();
  await flush();
  return pending.length;
}

test('pgnQueueLineEvaluations: searches every board the graph is drawn from, not just the book\'s', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  // The book holds the opening: give the first ply's children a server evaluation.
  for (const s of game.pgnChildrenByPly[0]) {
    game.evaluations.set(s, { board: s, score: 0, source: 'edax', level: MAX_TARGET_LEVEL });
  }

  game.pgnQueueLineEvaluations();

  const searched = new Set(pool.calls.map((c) => c.player + ':' + c.opponent));
  const expected = game.pgnAllChildStrings.filter((s) => !game.evaluations.has(s));
  assert.equal(searched.size, expected.length, 'one search per line board the server has not answered for');
  assert.ok(pool.calls.every((c) => c.level === 4), 'the whole line starts at the shallowest level');
  assert.ok(pool.calls.every((c) => c.tag === game._localEvalLineTag()), 'tagged as line work, not board work');
});

test('pgnQueueLineEvaluations: local scores complete the graph over plies the book has no evaluation for', async () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  game.edaxWorkerPool = mockWorkerPool();

  assert.ok(game.pgnGetGraphData().every((d) => d === null), 'nothing to draw before any evaluation');

  game.pgnQueueLineEvaluations();
  await resolveAll(game.edaxWorkerPool, 5);

  const data = game.pgnGetGraphData();
  for (let ply = 0; ply < game.pgnBoards.length; ply++) {
    const board = game.pgnBoards[ply];
    if (!board.hasValidMoves()) {
      assert.equal(data[ply], null, `ply ${ply} has no move to score`);
      continue;
    }
    assert.ok(data[ply], `ply ${ply} is on the graph`);
    // Every child scored 5 from its own mover's POV, so the ply's score is 5 for whoever is to
    // move there, converted to black's POV.
    assert.equal(data[ply].score, board.blackTurn ? -5 : 5);
  }
});

test('pgnQueueLineEvaluations: stepping to another ply keeps the rest of the graph\'s searches', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  game.pgnQueueLineEvaluations();
  const queued = pool.calls.length;
  assert.ok(queued > 0);

  // Arrow-right: pgnRenderCurrentPly asks for the newly displayed board's moves, which bumps the
  // per-board generation and drops that board's stale work.
  game.pgnCurrentPly = 5;
  game.requestMissingEvaluations(game.pgnDisplayBoardOriented());

  assert.ok(pool.calls.every((c) => !c.resolved), 'no line search was cancelled');
  assert.equal(pool.calls.length, queued, 'and the displayed ply queues nothing twice — the line already covers it');
});

test('pgnQueueLineEvaluations: a line board dropped as some other position\'s work is picked back up', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  // A board of the line queued as the displayed position's own work (as requestMissingEvaluations
  // does for a board reached by exploring off the line), before the line claimed it.
  const boardStr = game.pgnAllChildStrings[0];
  game.queueLocalEvaluations([boardStr]);
  game.pgnQueueLineEvaluations();
  const dropped = pool.calls[0];
  assert.notEqual(dropped.tag, game._localEvalLineTag(), 'it belongs to the board, not the line');

  game.pgnStepPly(1);

  assert.ok(dropped.resolved, 'stepping to another ply cancelled it');
  const board = OthelloBoard.fromString(boardStr);
  const resumed = pool.calls.filter((c) => !c.resolved && c.player === board.playerBits && c.opponent === board.opponentBits);
  assert.equal(resumed.length, 1, 'and the line queued it again rather than leaving the graph point stuck');
  assert.equal(resumed[0].tag, game._localEvalLineTag());
});

test('pgnAbandonLineEvaluations: loading another PGN drops the previous line\'s queued searches', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const pool = mockWorkerPool();
  game.edaxWorkerPool = pool;

  game.pgnQueueLineEvaluations();
  assert.ok(pool.calls.length > 0);

  game.pgnAbandonLineEvaluations();

  assert.ok(pool.calls.every((c) => c.resolved), 'every queued search was cancelled');
  assert.equal(game._pendingLocalEvals.size, 0, 'and their boards are free to be queued again');
});

test('pgnRequestLevelUps: a local wasm score is not a rung on the server\'s level ladder', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const requests = [];
  game.wsClient = { requestEvaluations() {}, sendEvent(event, boards, level) { requests.push({ event, boards, level }); } };

  // The state right after loading: the server was asked at the priority level and has not answered
  // yet, while the local evaluator has already refined a board to level 8.
  const boardStr = game.pgnAllChildStrings[0];
  for (const s of game.pgnAllChildStrings) game.pendingLevelRequests.set(s, game.levelConfig.priorityLevel);
  game.evaluations.set(boardStr, { board: boardStr, score: 1, source: 'wasm', level: 8 });

  game.pgnRequestLevelUps();

  assert.deepEqual(requests, [], 'no request: stepping up from level 8 would ask for a shallower search than level 10');

  // Once the server does answer, the ladder climbs from *its* level.
  game.evaluations.set(boardStr, { board: boardStr, score: 1, source: 'edax', level: 10 });
  game.pgnRequestLevelUps();

  assert.deepEqual(requests, [{ event: 'analyze_request', boards: [boardStr], level: 12 }]);
});

test('a local result redraws the score graph, not just the board overlay', async () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  game.edaxWorkerPool = mockWorkerPool();
  let graphRenders = 0;
  game.pgnRenderGraph = () => { graphRenders++; };

  game.pgnQueueLineEvaluations();
  await resolveAll(game.edaxWorkerPool, 5);

  assert.ok(graphRenders > 0, 'the graph is redrawn as local scores arrive');
});
