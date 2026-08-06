// Test harness: makes the browser-only static/board.js loadable under Node so its pure logic
// can be exercised. board.js bootstraps the page with `new OthelloGame()` only when there is no
// CommonJS `module` (i.e. in the browser); under Node it instead exports its classes. We install
// a handful of no-op DOM stubs as globals so the class's render methods can run without a real
// document/canvas — the tests never assert on rendered pixels, only on the pure data
// (pgnGetGraphData, pgnIsForcedPass, pgnStepPly navigation, OthelloBoard).

// ── Minimal DOM stubs ────────────────────────────────────────────────────────
function stubElement() {
  const noop = () => {};
  return {
    style: {},
    dataset: {},
    classList: { toggle: noop, add: noop, remove: noop, contains: () => false },
    addEventListener: noop,
    appendChild: noop,
    focus: noop,
    querySelectorAll: () => [],
    getBoundingClientRect: () => ({ width: 400, height: 200, left: 0, top: 0 }),
    getContext: () => new Proxy({}, { get: () => () => {} }),
    innerHTML: '',
    textContent: '',
    width: 400,
    height: 200,
  };
}

global.document = {
  getElementById: () => stubElement(),
  createElement: () => stubElement(),
  querySelector: () => stubElement(),
  querySelectorAll: () => [],
  addEventListener: () => {},
};
global.window = { location: { protocol: 'http:', host: 'localhost' }, devicePixelRatio: 1 };
global.getComputedStyle = () => ({ getPropertyValue: () => '' });
global.WebSocket = function () { this.send = () => {}; };
global.WebSocket.OPEN = 1;
global.fetch = () => Promise.reject(new Error('no network in tests'));
global.requestAnimationFrame = (cb) => setTimeout(cb, 0);

// ── Load the real classes ────────────────────────────────────────────────────
const { OthelloBoard, OthelloGame, LOCAL_EVAL_LEVELS, localEvalLevelsFor } = require('../board.js');

const DEFAULT_LEVEL_CONFIG = {
  priorityLevel: 10,
  maxSavableDiscs: 30,
  leafDiscs: 12,
  targetLevelNonLeaf: 16,
  targetLevelLeaf: 24,
};

// buildGame constructs an OthelloGame without its DOM-touching constructor, wiring up the same
// PGN-review state analyzePGN()/pgnBuildChildSets() would, already in pgnState 'graph'. With
// `complete` (default), every child board is given a deterministic evaluation so
// pgnGetGraphData() yields a full line, mirroring the "analysis complete" state.
function buildGame(boardStrings, { complete = true } = {}) {
  const game = Object.create(OthelloGame.prototype);
  game.pgnState = 'graph';
  game.pgnBoards = boardStrings.map((s) => OthelloBoard.fromString(s));
  game.evaluations = new Map();
  game.pendingLevelRequests = new Map();
  game.pgnCurrentPly = 0;
  game.pgnAlternativeMoves = [];
  game.flipped = false;
  game.evalMode = true;
  game.levelConfig = { ...DEFAULT_LEVEL_CONFIG };
  game.pgnPollTimer = null; // tests don't wait on polling; pgnRequestDivergedEvals may start it
  // No-op by default; tests that need to inspect outgoing requests replace this.
  game.wsClient = { requestEvaluations() {}, sendEvent() {} };
  game._graphData = null;
  game._graphLayout = null;
  game._graphClickBound = false;
  game.edaxWorkerPool = null;
  game._pendingLocalEvals = new Set();
  game._localEvalRenderPending = false;
  game._localEvalBoardKey = null;
  game._localEvalGeneration = 0;

  // Mirror pgnBuildChildSets(): children of each valid-move board, [] for pass/game-over.
  game.pgnChildrenByPly = game.pgnBoards.map((b) =>
    b.hasValidMoves() ? b.getChildren().map((c) => c.normalize().toString()) : []);
  const seen = new Set();
  for (const children of game.pgnChildrenByPly) for (const s of children) seen.add(s);
  game.pgnAllChildStrings = [...seen];

  if (complete) {
    let i = 0;
    for (const s of game.pgnAllChildStrings) {
      // Deterministic pseudo-scores in [-10, 10]; content is irrelevant to the tests,
      // only that every child has a known evaluation at the leaf target level.
      game.evaluations.set(s, { board: s, score: ((i++ * 7) % 21) - 10, source: 'edax', level: 24 });
    }
  }
  return game;
}

// graphSegments reproduces board.js pgnRenderGraph()'s score-line loop: it walks the
// pgnGetGraphData() output, skipping null entries without breaking the path, and returns
// the list of [from, to] index pairs that would be stroked. A forced-pass ply p is
// "bridged" iff some returned segment [a, b] has a < p < b.
function graphSegments(data) {
  const segs = [];
  let prev = -1;
  for (let i = 0; i < data.length; i++) {
    if (!data[i]) continue;
    if (prev !== -1) segs.push([prev, i]);
    prev = i;
  }
  return segs;
}

// buildNormalGame constructs an OthelloGame in normal (non-PGN) play mode, bypassing the
// DOM-touching constructor the same way buildGame does. board defaults to the starting position.
function buildNormalGame(board = new OthelloBoard()) {
  const game = Object.create(OthelloGame.prototype);
  game.pgnState = null;
  game.evaluations = new Map();
  game.board = board;
  game.boardHistory = [];
  game.evalMode = true;
  game.evalPollTimer = null;
  game.evalPollStart = 0;
  game.levelConfig = { ...DEFAULT_LEVEL_CONFIG };
  game.pendingLevelRequests = new Map();
  game._pendingLocalEvals = new Set();
  game._localEvalRenderPending = false;
  game._localEvalBoardKey = null;
  game._localEvalGeneration = 0;
  game.edaxWorkerPool = null;
  // No-op by default; tests that need to inspect outgoing requests replace this.
  game.wsClient = { requestEvaluations() {}, sendEvent() {} };
  return game;
}

module.exports = {
  OthelloBoard,
  OthelloGame,
  buildGame,
  buildNormalGame,
  graphSegments,
  DEFAULT_LEVEL_CONFIG,
  LOCAL_EVAL_LEVELS,
  localEvalLevelsFor,
};
