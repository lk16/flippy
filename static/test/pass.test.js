// Tests for the forced-pass behaviors on the PGN review section of /game:
//  - the score graph bridges dot-less forced-pass plies without a gap,
//  - arrow-key navigation skips forced-pass plies but never the final game-over ply,
//  - navigation re-renders the graph so its current-ply highlight tracks the move.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame, graphSegments, OthelloPosition } = require('./harness');
const { FORCED_PASS_BOARDS, FORCED_PASS_PLIES, GAME_OVER_PLY } = require('./fixtures');

// Sanity: the fixture's structural facts hold (pins the embedded strings to real backend output).
test('fixture: forced-pass and game-over plies are what we expect', () => {
  const boards = FORCED_PASS_BOARDS.map((s) => OthelloPosition.fromString(s));
  boards.forEach((b, i) => assert.ok(b, `board ${i} parsed`));
  for (const p of FORCED_PASS_PLIES) {
    assert.equal(boards[p].hasValidMoves(), false, `ply ${p} has no moves`);
    assert.equal(boards[p].isGameOver(), false, `ply ${p} is not game over (forced pass)`);
  }
  assert.equal(boards[GAME_OVER_PLY].hasValidMoves(), false);
  assert.equal(boards[GAME_OVER_PLY].isGameOver(), true, 'final ply is game over');
});

// Pins static/board.js's move generation and encoding to the backend's: every ply of the fixture
// must come out of the previous one, string for string.
test('fixture: replaying it in JS reproduces every backend board string', () => {
  let board = OthelloPosition.fromString(FORCED_PASS_BOARDS[0]);

  for (let ply = 1; ply < FORCED_PASS_BOARDS.length; ply++) {
    let next;
    if (board.hasValidMoves()) {
      next = board.getChildren().find((c) => c.toString() === FORCED_PASS_BOARDS[ply]);
    } else {
      next = board.clone();
      next.passMove();
    }
    assert.ok(next, `ply ${ply} is reachable from ply ${ply - 1}`);
    assert.equal(next.toString(), FORCED_PASS_BOARDS[ply], `ply ${ply} matches`);
    board = next;
  }
});

test('pgnGetGraphData: forced-pass and game-over plies are null, real moves are not', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  const data = game.pgnGetGraphData();
  assert.equal(data.length, FORCED_PASS_BOARDS.length);
  const nulls = data.map((d, i) => (d === null ? i : -1)).filter((i) => i >= 0);
  assert.deepEqual(nulls, [...FORCED_PASS_PLIES, GAME_OVER_PLY].sort((a, b) => a - b));
  // Every non-null entry is a real scored point.
  data.forEach((d, i) => {
    if (d !== null) assert.equal(typeof d.score, 'number', `ply ${i} has a numeric score`);
  });
});

test('graph line bridges each forced-pass ply without a break', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  const segs = graphSegments(game.pgnGetGraphData());
  for (const p of FORCED_PASS_PLIES) {
    const bridged = segs.some(([a, b]) => a < p && b > p);
    assert.ok(bridged, `forced-pass ply ${p} is bridged by a drawn line segment`);
  }
});

test('pgnIsForcedPass: true only for forced passes, false for game over', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  const forced = game.pgnBoards.map((_, i) => i).filter((i) => game.pgnIsForcedPass(i));
  assert.deepEqual(forced, FORCED_PASS_PLIES);
  assert.equal(game.pgnIsForcedPass(GAME_OVER_PLY), false, 'final game-over ply is not a forced pass');
});

test('pgnStepPly forward skips forced passes, lands on the final game-over ply', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.pgnCurrentPly = 0;
  const landings = [];
  for (let k = 0; k < FORCED_PASS_BOARDS.length + 5; k++) {
    const before = game.pgnCurrentPly;
    game.pgnStepPly(1);
    if (game.pgnCurrentPly === before) break;
    landings.push(game.pgnCurrentPly);
  }
  for (const p of FORCED_PASS_PLIES) {
    assert.ok(!landings.includes(p), `forward navigation never lands on forced-pass ply ${p}`);
  }
  assert.ok(landings.includes(GAME_OVER_PLY), 'forward navigation reaches the final game-over ply');
  assert.equal(game.pgnCurrentPly, GAME_OVER_PLY, 'navigation ends at the final ply');
});

test('pgnStepPly backward skips forced passes, reaches ply 0', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.pgnCurrentPly = GAME_OVER_PLY;
  const landings = [];
  for (let k = 0; k < FORCED_PASS_BOARDS.length + 5; k++) {
    const before = game.pgnCurrentPly;
    game.pgnStepPly(-1);
    if (game.pgnCurrentPly === before) break;
    landings.push(game.pgnCurrentPly);
  }
  for (const p of FORCED_PASS_PLIES) {
    assert.ok(!landings.includes(p), `backward navigation never lands on forced-pass ply ${p}`);
  }
  assert.ok(landings.includes(0), 'backward navigation reaches ply 0');
});

test('navigation re-renders the graph so the highlight tracks the current ply', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  const rendered = [];
  const real = Object.getPrototypeOf(game).pgnRenderGraph;
  game.pgnRenderGraph = function () { rendered.push(this.pgnCurrentPly); return real.call(this); };
  game.pgnCurrentPly = 0;
  const landings = [];
  for (let k = 0; k < 10; k++) {
    const before = game.pgnCurrentPly;
    game.pgnStepPly(1);
    if (game.pgnCurrentPly === before) break;
    landings.push(game.pgnCurrentPly);
  }
  assert.deepEqual(rendered, landings, 'each navigation triggered a graph render at the landed ply');
});

test('pgnGoBack/pgnGoForward step by one PGN ply while not diverged', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.pgnCurrentPly = 10;
  game.pgnGoForward();
  assert.equal(game.pgnCurrentPly, 11);
  game.pgnGoBack();
  assert.equal(game.pgnCurrentPly, 10);
});

test('pgnRenderGraph draws one continuous line across forced-pass plies (no path break)', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.pgnCurrentPly = 0;

  // Record every beginPath/moveTo/lineTo call (pgnRenderGraph draws several paths: gridlines,
  // the zero line, the score line, plus per-dot arcs). Grouping by beginPath and picking the
  // group with the most lineTo calls isolates the score line unambiguously — it has one lineTo
  // per known-score ply, far more than any gridline segment.
  const events = [];
  const ctx = new Proxy({}, {
    get(_, prop) {
      if (prop === 'moveTo' || prop === 'lineTo' || prop === 'beginPath') {
        return () => events.push(prop);
      }
      return () => {};
    },
  });
  const canvas = {
    style: {},
    getBoundingClientRect: () => ({ width: 400, height: 200, left: 0, top: 0 }),
    getContext: () => ctx,
    addEventListener: () => {},
  };
  const real = document.getElementById;
  document.getElementById = (id) => (id === 'score-graph' ? canvas : real(id));
  try {
    game.pgnRenderGraph();
  } finally {
    document.getElementById = real;
  }

  const segments = [];
  for (const kind of events) {
    if (kind === 'beginPath') { segments.push([]); continue; }
    if (segments.length) segments[segments.length - 1].push(kind);
  }
  const scoreLineSegment = segments.reduce((best, seg) => (
    seg.filter((k) => k === 'lineTo').length > best.filter((k) => k === 'lineTo').length ? seg : best
  ), []);

  // A single moveTo starts the path; every other known-score point is a lineTo. Under the old
  // bug a forced-pass/game-over gap started a fresh moveTo, breaking the line into segments.
  const moveTos = scoreLineSegment.filter((k) => k === 'moveTo');
  const lineTos = scoreLineSegment.filter((k) => k === 'lineTo');
  assert.equal(moveTos.length, 1, 'the score line is drawn as a single unbroken path');
  const knownCount = game.pgnGetGraphData().filter(Boolean).length;
  assert.equal(lineTos.length, knownCount - 1, 'one lineTo per known-score point after the first');
});
