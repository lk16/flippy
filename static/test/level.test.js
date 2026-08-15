// Tests for incremental level-up analysis: target levels, "at target" completion tracking,
// eval dedup that never downgrades a cached result, and the batched level-up requests that
// drive analysis from the priority level up to each board's target.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame, OthelloGame, DEFAULT_LEVEL_CONFIG, MAX_TARGET_LEVEL } = require('./harness');
const { FORCED_PASS_BOARDS } = require('./fixtures');

test('targetLevelForBoard: picks the tier the board\'s disc count falls in', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  // FORCED_PASS_BOARDS is one board per ply, starting at 4 discs, so ply n has n + 4 discs.
  assert.equal(game.targetLevelForBoard(game.pgnBoards[0].normalize().toString()), 40, '4 discs -> first tier');
  assert.equal(game.targetLevelForBoard(game.pgnBoards[9].normalize().toString()), 40, '13 discs -> still first tier');
  assert.equal(game.targetLevelForBoard(game.pgnBoards[10].normalize().toString()), 36, '14 discs -> second tier');
  assert.equal(game.targetLevelForBoard(game.pgnBoards[12].normalize().toString()), 36, '16 discs -> still second tier');
  assert.equal(game.targetLevelForBoard(game.pgnBoards[13].normalize().toString()), 34, '17 discs -> third tier');
  assert.equal(game.targetLevelForBoard(game.pgnBoards[16].normalize().toString()), 34, '20 discs -> still third tier');
  assert.equal(game.targetLevelForBoard(game.pgnBoards[17].normalize().toString()), 32, '21 discs -> last tier');
  assert.equal(game.targetLevelForBoard(game.pgnBoards[20].normalize().toString()), 32, '24 discs -> last tier');
});

test('targetLevelForBoard: boards past maxSavableDiscs use the target for exactly that many discs', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  // Mirrors api.EffectiveTargetLevel, which clamps the disc count before picking a tier.
  const deep = game.pgnBoards[50].normalize().toString(); // 54 discs, well past maxSavableDiscs (30)
  assert.equal(game.targetLevelForBoard(deep), game.targetLevelForBoard(game.pgnBoards[26].normalize().toString()));
});

test('isAtTarget: false with no evaluation, false below target, true at/above target', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const s = game.pgnBoards[40].normalize().toString();
  const target = game.targetLevelForBoard(s);
  assert.equal(game.isAtTarget(s), false, 'no evaluation yet');

  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: target - 2 });
  assert.equal(game.isAtTarget(s), false, 'below target level');

  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: target });
  assert.equal(game.isAtTarget(s), true, 'at target level');
});

test('isAtTarget: minimax/final results are always at target regardless of level', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const s = game.pgnBoards[40].normalize().toString();
  game.evaluations.set(s, { board: s, score: 1, source: 'final', level: 0 });
  assert.equal(game.isAtTarget(s), true);
});

test('isAtTarget: a search that ran the game out is at target whatever its level', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const s = game.pgnBoards[40].normalize().toString(); // 44 discs, so 20 empties
  const target = game.targetLevelForBoard(s);

  // Level 10 solves all 20 empties outright: depth 20 at full width, which no level can improve on.
  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: 10, depth: 20, confidence: 100 });
  assert.ok(10 < target, 'the level is below target, so only the search itself can end the climb');
  assert.equal(game.isAtTarget(s), true);

  // Same depth but selective: the search did not run the game out at full width.
  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: 10, depth: 20, confidence: 98 });
  assert.equal(game.isAtTarget(s), false);

  // Deep, but short of the game's end.
  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: 10, depth: 18, confidence: 100 });
  assert.equal(game.isAtTarget(s), false);
});

test('pgnRequestLevelUps: does not climb past a search that ran the game out', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const sent = [];
  game.wsClient = { sendEvent: (event, boards, level) => sent.push({ event, boards, level }) };

  for (const s of game.pgnAllChildStrings) {
    const empties = 64 - game.discCountFromBoardStr(s);
    if (empties === 0) continue; // a full board is game over, not a search result
    game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: 10, depth: empties, confidence: 100 });
  }
  game.pgnRequestLevelUps();

  assert.deepEqual(sent, []);
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
  game.evaluations.set(a, { board: a, score: 1, source: 'edax', level: MAX_TARGET_LEVEL });
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

test('pgnRequestLevelUps: the last step lands exactly on the target, never past it', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const s = game.pgnAllChildStrings.find((b) => game.targetLevelForBoard(b) % 2 === 0);
  const target = game.targetLevelForBoard(s);
  game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: target - 1 });

  const sent = [];
  game.wsClient = { requestEvaluations() {}, sendEvent(event, boards, level) { sent.push({ event, boards, level }); } };
  game.pgnRequestLevelUps();

  const req = sent.find((m) => m.boards.includes(s));
  assert.ok(req, 'a level-up request was sent for the below-target board');
  assert.equal(req.level, target, 'capped at the target rather than current + 2');
  assert.equal(game.pendingLevelRequests.get(s), target);
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
      target_levels: [
        { max_discs: 13, level: 40 },
        { max_discs: 16, level: 36 },
        { max_discs: 20, level: 34 },
        { max_discs: 64, level: 32 },
      ],
    }),
  });
  try {
    await game.fetchLevelConfig();
  } finally {
    global.fetch = originalFetch;
  }
  assert.deepEqual(game.levelConfig, DEFAULT_LEVEL_CONFIG);
});

test('fetchLevelConfig\'s offline fallback matches what the backend serves', async () => {
  // A fallback that targets deeper than the backend is willing to search would leave every board
  // permanently short of isAtTarget, so the PGN page would never report completion.
  const game = Object.create(OthelloGame.prototype);
  game.levelConfig = null;
  await game.fetchLevelConfig(); // harness stubs global.fetch to reject
  assert.deepEqual(game.levelConfig, DEFAULT_LEVEL_CONFIG);
});

// graphStatus runs pgnUpdateGraphStatus with #graph-status stubbed out and returns its text.
function graphStatus(game) {
  const statusEl = { textContent: '' };
  const real = document.getElementById;
  document.getElementById = (id) => (id === 'graph-status' ? statusEl : real(id));
  try {
    game.pgnUpdateGraphStatus();
  } finally {
    document.getElementById = real;
  }
  return statusEl.textContent;
}

test('pgnUpdateGraphStatus: reports the current (lowest) search level and progress', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const [a] = game.pgnAllChildStrings;
  // a is ahead at level 12; every other board is still untouched, so implicitly at the
  // priority level (10) — the reported level should be the minimum across all of them.
  game.evaluations.set(a, { board: a, score: 1, source: 'edax', level: 12 });
  game.pendingLevelRequests.set(a, 12);

  assert.equal(graphStatus(game), `Searching at level 10 — 0 / ${game.pgnAllChildStrings.length} boards evaluated…`);
});

test('pgnUpdateGraphStatus: boards already at target do not hold the reported level back', () => {
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  const total = game.pgnAllChildStrings.length;
  // Every board but the last is finished — and keeps its last requested level (the priority
  // level) in pendingLevelRequests forever. Only the unfinished one, now being searched at 16,
  // says anything about how deep the search currently is.
  for (const s of game.pgnAllChildStrings.slice(0, -1)) {
    game.evaluations.set(s, { board: s, score: 1, source: 'edax', level: MAX_TARGET_LEVEL });
    game.pendingLevelRequests.set(s, game.levelConfig.priorityLevel);
  }
  const last = game.pgnAllChildStrings[total - 1];
  game.evaluations.set(last, { board: last, score: 1, source: 'edax', level: 14 });
  game.pendingLevelRequests.set(last, 16);

  assert.equal(graphStatus(game), `Searching at level 16 — ${total - 1} / ${total} boards evaluated…`);
});

test('pgnUpdateGraphStatus: reports completion once every board is at target', () => {
  const game = buildGame(FORCED_PASS_BOARDS); // complete: true — every child at its target level
  assert.equal(graphStatus(game), 'Analysis complete.');
});
