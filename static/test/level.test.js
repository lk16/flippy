// Tests for incremental level-up analysis: target levels, "at target" completion tracking,
// eval dedup that never downgrades a cached result, and the batched level-up requests that
// drive analysis from the priority level up to each board's target.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame, OthelloGame, DEFAULT_LEVEL_CONFIG } = require('./harness');
const { FORCED_PASS_BOARDS } = require('./fixtures');

test('targetLevelForBoard: leaf boards target targetLevelLeaf, non-leaf target targetLevelNonLeaf', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  // Ply 0 (starting position, 4 discs) is well under leafDiscs (12) -> leaf target.
  const leafStr = game.pgnBoards[0].normalize().toString();
  assert.equal(game.targetLevelForBoard(leafStr), DEFAULT_LEVEL_CONFIG.targetLevelLeaf);
  // Ply 40 has far more than 12 discs -> non-leaf target.
  const nonLeafStr = game.pgnBoards[40].normalize().toString();
  assert.equal(game.targetLevelForBoard(nonLeafStr), DEFAULT_LEVEL_CONFIG.targetLevelNonLeaf);
});

test('isAtTarget: false with no evaluation, false below target, true at/above target', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const s = game.pgnBoards[40].normalize().toString(); // non-leaf, target 16
  assert.equal(game.isAtTarget(s), false, 'no evaluation yet');

  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: 10 });
  assert.equal(game.isAtTarget(s), false, 'below target level');

  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: 16 });
  assert.equal(game.isAtTarget(s), true, 'at target level');
});

test('isAtTarget: minimax/final results are always at target regardless of level', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const s = game.pgnBoards[40].normalize().toString();
  game.evaluations.set(s, { board: s, score: 1, source: 'final', level: 0 });
  assert.equal(game.isAtTarget(s), true);
});

test('shouldUpdateEval: never downgrades minimax/final to edax; edax only upgrades on higher level', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const final = { source: 'final', score: 5, level: 0 };
  const edaxLow = { source: 'edax', score: 1, level: 8 };
  const edaxHigh = { source: 'edax', score: 2, level: 16 };

  assert.equal(game.shouldUpdateEval(undefined, edaxLow), true, 'first evaluation always accepted');
  assert.equal(game.shouldUpdateEval(final, edaxLow), false, 'edax never downgrades a final result');
  assert.equal(game.shouldUpdateEval(edaxLow, edaxHigh), true, 'higher edax level wins');
  assert.equal(game.shouldUpdateEval(edaxHigh, edaxLow), false, 'lower edax level loses');
  assert.equal(game.shouldUpdateEval(edaxLow, final), true, 'final always wins over edax');
});

test('pgnUnresolved: excludes boards at target, includes boards below target or unevaluated', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const [a, b] = game.pgnAllChildStrings;
  game.evaluations.set(a, { board: a, score: 1, source: 'edax', level: 24 });
  // b left unevaluated.
  const unresolved = game.pgnUnresolved();
  assert.ok(!unresolved.includes(a), 'evaluated-at-target board is resolved');
  assert.ok(unresolved.includes(b), 'unevaluated board is unresolved');
});

test('pgnRequestLevelUps: requests the next level (+2) for boards below target, batched by level', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const nonLeaf = game.pgnBoards[40].getChildren().map((c) => c.normalize().toString())
    .filter((s) => game.pgnAllChildStrings.includes(s))[0];
  assert.ok(nonLeaf, 'found a non-leaf child board to test with');

  game.evaluations.set(nonLeaf, { board: nonLeaf, score: 1, source: 'edax', level: 10 });
  const sent = [];
  game.wsClient = { requestEvaluations() {}, sendEvent(event, boards, level) { sent.push({ event, boards, level }); } };

  game.pgnRequestLevelUps();

  const req = sent.find((m) => m.boards.includes(nonLeaf));
  assert.ok(req, 'a level-up request was sent for the below-target board');
  assert.equal(req.level, 12, 'requested level is current (10) + 2');
  assert.equal(game.pendingLevelRequests.get(nonLeaf), 12, 'pendingLevelRequests tracks the new level');
});

test('pgnRequestLevelUps: does not re-request a level already pending', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const s = game.pgnAllChildStrings[0];
  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: 10 });
  game.pendingLevelRequests.set(s, 12); // already requested the next level

  const sent = [];
  game.wsClient = { requestEvaluations() {}, sendEvent(event, boards, level) { sent.push({ event, boards, level }); } };
  game.pgnRequestLevelUps();

  assert.ok(!sent.some((m) => m.boards.includes(s)), 'no duplicate request for an already-pending level');
});

test('pgnRequestLevelUps: never re-requests minimax/final results', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const s = game.pgnAllChildStrings[0];
  game.evaluations.set(s, { board: s, score: 1, source: 'final', level: 0 });

  const sent = [];
  game.wsClient = { requestEvaluations() {}, sendEvent(event, boards, level) { sent.push({ event, boards, level }); } };
  game.pgnRequestLevelUps();

  assert.ok(!sent.some((m) => m.boards.includes(s)), 'final results are never leveled up further');
});

test('fetchLevelConfig maps the backend\'s snake_case JSON to the camelCase fields the rest of the code reads', async () => {
  const game = Object.create(OthelloGame.prototype);
  game.levelConfig = null;
  const originalFetch = global.fetch;
  global.fetch = async () => ({
    ok: true,
    json: async () => ({
      priority_level: 10,
      max_savable_discs: 30,
      leaf_discs: 12,
      target_level_leaf: 24,
      target_level_non_leaf: 16,
    }),
  });
  try {
    await game.fetchLevelConfig();
  } finally {
    global.fetch = originalFetch;
  }
  assert.deepEqual(game.levelConfig, {
    priorityLevel: 10,
    maxSavableDiscs: 30,
    leafDiscs: 12,
    targetLevelLeaf: 24,
    targetLevelNonLeaf: 16,
  });
});

test('pgnUpdateGraphStatus: reports the current (lowest) search level and progress', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const [a] = game.pgnAllChildStrings;
  // a is ahead at level 12; every other board is still untouched, so implicitly at the
  // priority level (10) — the reported level should be the minimum across all of them.
  game.evaluations.set(a, { board: a, score: 1, source: 'edax', level: 12 });
  game.pendingLevelRequests.set(a, 12);

  const statusEl = { textContent: '' };
  const real = document.getElementById;
  document.getElementById = (id) => (id === 'graph-status' ? statusEl : real(id));
  try {
    game.pgnUpdateGraphStatus();
  } finally {
    document.getElementById = real;
  }
  assert.equal(statusEl.textContent, `Searching at level 10 — 0 / ${game.pgnAllChildStrings.length} boards evaluated…`);
});

test('pgnUpdateGraphStatus: reports completion once every board is at target', () => {
  const game = buildGame(FORCED_PASS_BOARDS); // complete: true — every child at level 24
  const statusEl = { textContent: '' };
  const real = document.getElementById;
  document.getElementById = (id) => (id === 'graph-status' ? statusEl : real(id));
  try {
    game.pgnUpdateGraphStatus();
  } finally {
    document.getElementById = real;
  }
  assert.equal(statusEl.textContent, 'Analysis complete.');
});
