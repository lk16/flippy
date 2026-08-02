// Tests for the click-to-diverge feature on /game's PGN review section (ports pgn.py PGNMode's
// alternative_moves stack): clicking the PGN's played move advances the line; any other legal
// move diverges; undo pops the stack; forward is a no-op while diverged; a diverged move forcing
// a pass auto-advances; and the score graph's data is untouched by a divergence excursion.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame, OthelloBoard } = require('./harness');
const { FORCED_PASS_BOARDS } = require('./fixtures');

// recordingGame wires up a fake wsClient that records what pgnRequestDivergedEvals sends.
function recordingGame() {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.wsClient = {
    sent: [],
    requestEvaluations(boards) {
      if (boards.length) this.sent.push({ event: 'evaluation_request', boards, level: undefined });
    },
    sendEvent(event, boards, level) {
      if (boards.length) this.sent.push({ event, boards, level });
    },
  };
  return game;
}

// firstMoveTo returns the square index whose move on `board` yields `target` (or -1).
function firstMoveTo(board, target) {
  for (let i = 0; i < 64; i++) {
    const child = board.doMove(i);
    if (child && child.toString() === target.toString()) return i;
  }
  return -1;
}

// firstLegalDivergentMove returns a legal square on `board` whose result is NOT `avoid` (or -1).
function firstLegalDivergentMove(board, avoid) {
  for (let i = 0; i < 64; i++) {
    const child = board.doMove(i);
    if (child && (!avoid || child.toString() !== avoid.toString())) return i;
  }
  return -1;
}

test('clicking the move actually played in the PGN advances the line without diverging', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  const played = firstMoveTo(game.pgnBoards[0], game.pgnBoards[1]);
  assert.ok(played >= 0, 'found the played move index at ply 0');
  game.pgnOnSquareClick(played);
  assert.equal(game.pgnIsDiverged(), false, 'clicking the played move does not diverge');
  assert.equal(game.pgnCurrentPly, 1, 'the ply pointer advanced like arrow-right');
  assert.equal(game.wsClient.sent.length, 0, 'no diverged evaluation was requested');
});

test('clicking the played move whose result is a forced pass advances like next (skips the pass)', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 54; // pgnBoards[55] is a forced pass; pgnBoards[56] is the post-pass position
  const played = firstMoveTo(game.pgnBoards[54], game.pgnBoards[55]);
  assert.ok(played >= 0, 'found the pass-forcing played move at ply 54');
  game.pgnOnSquareClick(played);
  assert.equal(game.pgnIsDiverged(), false);
  assert.equal(game.pgnCurrentPly, 56, 'navigation skipped the forced-pass ply and landed post-pass');
});

test('clicking any other legal move diverges (pushes onto the stack)', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  const other = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
  assert.ok(other >= 0, 'found a legal move that leaves the line');
  const expected = game.pgnBoards[0].doMove(other);
  game.pgnOnSquareClick(other);
  assert.equal(game.pgnIsDiverged(), true, 'now diverged');
  assert.equal(game.pgnAlternativeMoves.length, 1);
  assert.equal(game.pgnDisplayBoard().toString(), expected.toString(), 'shows the explored board');
  assert.equal(game.pgnCurrentPly, 0, 'PGN ply pointer stays frozen at the divergence point');
  // Evaluations were requested for the diverged board via the normal pipeline.
  const evReq = game.wsClient.sent.find((m) => m.event === 'evaluation_request');
  const anReq = game.wsClient.sent.find((m) => m.event === 'analyze_request');
  assert.ok(evReq && anReq, 'both evaluation_request and analyze_request were sent');
  assert.ok(evReq.boards.includes(expected.normalize().toString()), 'diverged board is in the request');
});

test('undo (pgnGoBack) pops one diverged move and works mid-divergence', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
  game.pgnOnSquareClick(m1);
  const afterFirst = game.pgnDisplayBoard().toString();
  const m2 = firstLegalDivergentMove(game.pgnDisplayBoard(), null);
  game.pgnOnSquareClick(m2);
  assert.equal(game.pgnAlternativeMoves.length, 2, 'two moves deep');

  game.pgnGoBack();
  assert.equal(game.pgnAlternativeMoves.length, 1, 'undo popped one move (mid-divergence)');
  assert.equal(game.pgnDisplayBoard().toString(), afterFirst, 'back to the first explored board');

  game.pgnGoBack();
  assert.equal(game.pgnIsDiverged(), false, 'undo emptied the stack');
  assert.equal(game.pgnDisplayBoard().toString(), game.pgnBoards[0].toString(), 'back on the PGN line');
});

test('undoMove (right-click / Undo button) pops one diverged move while exploring', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
  game.pgnOnSquareClick(m1);
  assert.equal(game.pgnIsDiverged(), true, 'diverged after the first move');

  game.undoMove();
  assert.equal(game.pgnIsDiverged(), false, 'undoMove popped the only explored move');
  assert.equal(game.pgnDisplayBoard().toString(), game.pgnBoards[0].toString(), 'back on the PGN line');
});

test('undoMove steps back a PGN ply (like pgnGoBack) when not diverged', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 10;
  game.undoMove();
  assert.equal(game.pgnCurrentPly, 9, 'undoMove steps back one ply while on the PGN line');
});

test('forward (pgnGoForward) is a no-op while diverged', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
  game.pgnOnSquareClick(m1);
  const before = {
    ply: game.pgnCurrentPly,
    top: game.pgnDisplayBoard().toString(),
    depth: game.pgnAlternativeMoves.length,
  };
  game.pgnGoForward();
  assert.equal(game.pgnCurrentPly, before.ply, 'ply unchanged');
  assert.equal(game.pgnAlternativeMoves.length, before.depth, 'stack unchanged');
  assert.equal(game.pgnDisplayBoard().toString(), before.top, 'board unchanged');
});

test('a diverged move that forces a pass auto-advances through it', () => {
  const game = recordingGame();
  // Simulate being mid-exploration on a board identical to ply 54 (black to move), so the
  // pass-forcing move is treated as a divergence rather than a line match.
  game.pgnCurrentPly = 10; // frozen elsewhere; irrelevant to the divergence branch
  game.pgnAlternativeMoves = [OthelloBoard.fromString(FORCED_PASS_BOARDS[54])];
  const passForcing = firstMoveTo(game.pgnDisplayBoard(), OthelloBoard.fromString(FORCED_PASS_BOARDS[55]));
  assert.ok(passForcing >= 0, 'found the pass-forcing move');

  game.pgnOnSquareClick(passForcing);
  assert.equal(game.pgnAlternativeMoves.length, 2, 'pushed exactly one explored move');
  const top = game.pgnDisplayBoard();
  assert.equal(top.hasValidMoves(), true, 'auto-advanced past the pass onto a playable board');
  assert.equal(top.toString(), FORCED_PASS_BOARDS[56], 'landed on the post-pass position');
});

test('the score graph data is byte-identical before, during, and after a divergence excursion', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 5;
  const before = JSON.stringify(game.pgnGetGraphData());

  const m1 = firstLegalDivergentMove(game.pgnBoards[5], game.pgnBoards[6]);
  game.pgnOnSquareClick(m1);
  const during = JSON.stringify(game.pgnGetGraphData());

  game.pgnGoBack(); // return to the PGN line
  const after = JSON.stringify(game.pgnGetGraphData());

  assert.equal(during, before, 'graph data unchanged while diverged');
  assert.equal(after, before, 'graph data unchanged after the excursion');
  assert.equal(game.pgnIsDiverged(), false, 'excursion left the stack empty');
});

test('onCellClick delegates to pgnOnSquareClick while reviewing a PGN line', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  const played = firstMoveTo(game.pgnBoards[0], game.pgnBoards[1]);
  game.onCellClick(played);
  assert.equal(game.pgnCurrentPly, 1, 'clicking the board cell advanced the PGN line via the dispatcher');
});

test('clicking the graph while diverged returns to the PGN line', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  game.pgnRenderGraph(); // populate _graphData/_graphLayout for hit-testing
  const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
  game.pgnOnSquareClick(m1);
  assert.equal(game.pgnIsDiverged(), true);
  // Simulate a click near ply 3 on the graph.
  const { xScale } = game._graphLayout;
  game.onGraphClick({ clientX: xScale(3), clientY: 0 });
  assert.equal(game.pgnIsDiverged(), false, 'graph click cleared the divergence');
  assert.equal(game.pgnCurrentPly, 3, 'jumped to the clicked PGN ply');
});

// withTrackedBoardEl temporarily replaces document.getElementById('board') with a stub that
// records every classList.toggle call, runs fn, then restores the real stub.
function withTrackedBoardEl(fn) {
  const boardEl = { classList: { calls: [], toggle(cls, val) { this.calls.push([cls, !!val]); } } };
  const real = document.getElementById;
  document.getElementById = (id) => (id === 'board' ? boardEl : real(id));
  try {
    return fn(boardEl);
  } finally {
    document.getElementById = real;
  }
}

test('the board is not marked "exploring" while on the PGN line, but is once diverged', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  withTrackedBoardEl((boardEl) => {
    game.pgnRenderCurrentPly();
    assert.deepEqual(boardEl.classList.calls.at(-1), ['exploring', false], 'not exploring on the PGN line');

    const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
    game.pgnOnSquareClick(m1);
    assert.deepEqual(boardEl.classList.calls.at(-1), ['exploring', true], 'marked exploring once diverged');
  });
});

test('the game-status text is not altered while exploring (no "exploring" suffix)', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  const statusEl = { textContent: '' };
  const real = document.getElementById;
  document.getElementById = (id) => (id === 'game-status' ? statusEl : real(id));
  try {
    const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
    game.pgnOnSquareClick(m1);
    assert.ok(!statusEl.textContent.includes('exploring'), 'status text has no "exploring" suffix');
  } finally {
    document.getElementById = real;
  }
});

test('diverging requests evaluations for the explored board\'s children (for the on-board overlay)', () => {
  const game = recordingGame();
  game.pgnCurrentPly = 0;
  const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
  const explored = game.pgnBoards[0].doMove(m1);
  game.pgnOnSquareClick(m1);

  const childStrings = explored.getChildren().map((c) => c.normalize().toString());
  const evReq = game.wsClient.sent.find((m) => m.event === 'evaluation_request');
  const anReq = game.wsClient.sent.find((m) => m.event === 'analyze_request');
  for (const s of childStrings) {
    assert.ok(evReq.boards.includes(s), `evaluation_request covers explored child ${s}`);
    assert.ok(anReq.boards.includes(s), `analyze_request covers explored child ${s}`);
  }
});

test('diverging also prefetches grandchild evaluations, like normal play', () => {
  const game = recordingGame();
  game.evaluations.clear(); // recordingGame builds with complete: true; start from nothing known
  game.pgnCurrentPly = 0;
  const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
  const explored = game.pgnBoards[0].doMove(m1);
  game.pgnOnSquareClick(m1);

  const grandchildStrings = new Set();
  for (const child of explored.getChildren()) {
    for (const grandchild of child.getChildren()) {
      grandchildStrings.add(grandchild.normalize().toString());
    }
  }
  assert.ok(grandchildStrings.size > 0, 'the explored board has grandchildren to prefetch');

  const evRequests = game.wsClient.sent.filter((m) => m.event === 'evaluation_request');
  const requested = new Set(evRequests.flatMap((m) => m.boards));
  for (const s of grandchildStrings) {
    assert.ok(requested.has(s), `grandchild ${s} was requested for prefetch`);
  }
});

test('polling keeps re-requesting the explored subtree until it resolves (not just once)', () => {
  const game = recordingGame();
  game.evaluations.clear(); // recordingGame builds with complete: true; start from nothing known
  game.pgnCurrentPly = 0;
  const m1 = firstLegalDivergentMove(game.pgnBoards[0], game.pgnBoards[1]);
  game.pgnOnSquareClick(m1); // diverges; pgnRequestDivergedEvals starts polling since none was running
  assert.ok(game.pgnPollTimer, 'diverging (re)starts polling so a slow search still gets picked up');

  const exploreTargets = game.pgnExploreTargets();
  assert.ok(exploreTargets.length > 0, 'there are targets to poll for the explored board');
  assert.ok(exploreTargets.every((s) => !game.evaluations.has(s)), 'none are evaluated yet in this test');

  game.stopPGNPolling();
});
