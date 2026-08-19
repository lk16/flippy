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

// ── Cell-aware DOM ───────────────────────────────────────────────────────────
// The stubs above answer every query with an empty list, which is enough for the pure-data tests
// but hides *where* a render puts things. installCellDOM() swaps in a document with 64 real cell
// elements, so a test can assert which squares ended up with a score overlay — the difference
// between a score under a legal move and a score sitting on top of a disc.

class FakeElement {
  constructor() {
    this.children = [];
    this.parentNode = null;
    this.dataset = {};
    this.style = {};
    this.textContent = '';
    this.innerHTML = '';
    this.width = 400;
    this.height = 200;
    this._classes = new Set();
  }

  get className() { return [...this._classes].join(' '); }
  set className(value) { this._classes = new Set(String(value).split(/\s+/).filter(Boolean)); }

  get classList() {
    return {
      add: (...cs) => cs.forEach((c) => this._classes.add(c)),
      remove: (...cs) => cs.forEach((c) => this._classes.delete(c)),
      contains: (c) => this._classes.has(c),
      toggle: (c, force) => {
        const on = force === undefined ? !this._classes.has(c) : force;
        if (on) this._classes.add(c); else this._classes.delete(c);
      },
    };
  }

  appendChild(child) { child.parentNode = this; this.children.push(child); return child; }

  removeChild(child) {
    const i = this.children.indexOf(child);
    if (i >= 0) this.children.splice(i, 1);
    child.parentNode = null;
    return child;
  }

  remove() { if (this.parentNode) this.parentNode.removeChild(this); }

  querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }

  querySelectorAll(selector) { return matchElements(this.children, selector); }

  addEventListener() {}
  focus() {}
  getBoundingClientRect() { return { width: 400, height: 200, left: 0, top: 0 }; }
  getContext() { return new Proxy({}, { get: () => () => {} }); }
}

// matchElements supports exactly the selector shapes board.js uses: a class chain
// ('.cell.next-move-played'), a descendant pair ('.cell .score-display') and an attribute lookup
// ('.cell[data-index="12"]').
function matchElements(elements, selector) {
  const parts = selector.trim().split(/\s+/);
  if (parts.length > 1) {
    return matchElements(elements, parts[0]).flatMap((el) => matchElements(el.children, parts.slice(1).join(' ')));
  }
  const attr = /^(\.[\w-]+)\[data-index="(\d+)"\]$/.exec(selector);
  if (attr) {
    return matchElements(elements, attr[1]).filter((el) => el.dataset.index === attr[2]);
  }
  const classes = selector.split('.').filter(Boolean);
  const out = [];
  for (const el of elements) {
    if (classes.every((c) => el._classes.has(c))) out.push(el);
    out.push(...matchElements(el.children, selector));
  }
  return out;
}

// installCellDOM replaces global.document with one holding 64 '.cell' elements (data-index 0..63),
// as initializeBoard() builds in the browser. Call restore() afterwards so the cheap stubs are
// back for the other tests.
function installCellDOM() {
  const previous = global.document;
  const cells = [];
  for (let i = 0; i < 64; i++) {
    const cell = new FakeElement();
    cell.className = 'cell';
    cell.dataset.index = String(i);
    cells.push(cell);
  }
  const byId = new Map();

  global.document = {
    getElementById: (id) => {
      if (!byId.has(id)) byId.set(id, new FakeElement());
      return byId.get(id);
    },
    createElement: () => new FakeElement(),
    querySelector: (selector) => matchElements(cells, selector)[0] || null,
    querySelectorAll: (selector) => matchElements(cells, selector),
    addEventListener: () => {},
  };

  return {
    cells,
    // scoredIndices lists the squares currently showing a move score, in ascending order.
    scoredIndices: () => cells
      .filter((c) => c.querySelector('.score-display'))
      .map((c) => Number(c.dataset.index))
      .sort((a, b) => a - b),
    // discIndices lists the squares currently holding a disc.
    discIndices: () => cells
      .filter((c) => c.querySelector('.piece'))
      .map((c) => Number(c.dataset.index))
      .sort((a, b) => a - b),
    restore: () => { global.document = previous; },
  };
}

// ── Load the real classes ────────────────────────────────────────────────────
const { OthelloBoard, OthelloGame, LOCAL_EVAL_LEVELS, localEvalLevelsFor } = require('../board.js');

// Mirrors GET /api/level-config (internal/api/handlers.go handleLevelConfig).
const DEFAULT_LEVEL_CONFIG = {
  priorityLevel: 10,
  maxSavableDiscs: 30,
  targetLevels: [
    { maxDiscs: 13, level: 40 },
    { maxDiscs: 16, level: 36 },
    { maxDiscs: 20, level: 34 },
    { maxDiscs: 64, level: 32 },
  ],
};

// MAX_TARGET_LEVEL is the deepest target any board can have, so an evaluation at this level counts
// as "at target" whatever the board's disc count.
const MAX_TARGET_LEVEL = Math.max(...DEFAULT_LEVEL_CONFIG.targetLevels.map((t) => t.level));

// buildGame constructs an OthelloGame without its DOM-touching constructor, wiring up the same
// PGN-review state analyzePGN()/pgnBuildChildSets() would, already in pgnState 'graph'. With
// `complete` (default), every child board is given a deterministic evaluation so
// pgnGetGraphData() yields a full line, mirroring the "analysis complete" state.
function buildGame(boardStrings, { complete = true } = {}) {
  const game = Object.create(OthelloGame.prototype);
  game.pgnState = 'graph';
  // Mirror analyzePGN(): a PGN line starts with black to move and every ply hands the turn over.
  game.pgnBoards = boardStrings.map((s, ply) => OthelloBoard.fromString(s, ply % 2 === 0));
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
  game._pendingLocalEvals = new Map();
  game._localEvalRenderPending = false;
  game._localEvalBoardKey = null;
  game._localEvalGeneration = 0;
  game._localEvalLineGeneration = 0;

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
      // only that every child has a known evaluation at (or above) its target level.
      game.evaluations.set(s, { board: s, score: ((i++ * 7) % 21) - 10, source: 'edax', level: MAX_TARGET_LEVEL });
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
  game._pendingLocalEvals = new Map();
  game._localEvalRenderPending = false;
  game._localEvalBoardKey = null;
  game._localEvalGeneration = 0;
  game._localEvalLineGeneration = 0;
  game.edaxWorkerPool = null;
  // No-op by default; tests that need to inspect outgoing requests replace this.
  game.wsClient = { requestEvaluations() {}, sendEvent() {} };
  return game;
}

// mockWorkerPool stands in for EdaxEvalWorkerPool: it records every evaluate() call and lets the
// test resolve them one at a time, mirroring the real signature (player, opponent, level, options)
// -> Promise<score>. cancelQueued() drops every not-yet-resolved call whose tag matches, the same
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

// flush yields to the microtask/timer queue, letting promise chains under test settle.
function flush() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

module.exports = {
  OthelloBoard,
  OthelloGame,
  mockWorkerPool,
  flush,
  installCellDOM,
  buildGame,
  buildNormalGame,
  graphSegments,
  DEFAULT_LEVEL_CONFIG,
  MAX_TARGET_LEVEL,
  LOCAL_EVAL_LEVELS,
  localEvalLevelsFor,
};
