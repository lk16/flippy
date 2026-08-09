// Tests for the F-key board flip on /game's PGN review section: a purely visual 180° rotation
// (rotate(3): square i -> 63-i) that must not change evaluations, the score graph, or which move
// a click plays.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame, installCellDOM } = require('./harness');
const { FORCED_PASS_BOARDS } = require('./fixtures');

// validMoveIndices lists board's legal-move squares, i.e. exactly the cells a render should put a
// score on. Legal moves are always empty squares, so any score outside this set sits on a disc.
function validMoveIndices(board) {
  const moves = board.getValidMoves();
  const out = [];
  for (let i = 0; i < 64; i++) if (((1n << BigInt(i)) & moves) !== 0n) out.push(i);
  return out;
}

function firstMoveTo(board, target) {
  for (let i = 0; i < 64; i++) {
    const child = board.doMove(i);
    if (child && child.toString() === target.toString()) return i;
  }
  return -1;
}

test('pgnFlipBoard toggles the flip flag and is its own inverse', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  assert.equal(game.flipped, false);
  game.pgnFlipBoard();
  assert.equal(game.flipped, true);
  game.pgnFlipBoard();
  assert.equal(game.flipped, false);
});

test('flipping does not change the score graph data or any evaluations', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.pgnCurrentPly = 20;
  const graphBefore = JSON.stringify(game.pgnGetGraphData());
  const evalsBefore = JSON.stringify([...game.evaluations.entries()]);
  game.pgnFlipBoard();
  assert.equal(JSON.stringify(game.pgnGetGraphData()), graphBefore, 'graph data unchanged by flip');
  assert.equal(JSON.stringify([...game.evaluations.entries()]), evalsBefore, 'evaluations unchanged by flip');
});

test('flipping issues no new evaluation requests when children are already cached', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.pgnCurrentPly = 0;
  const sent = [];
  game.wsClient = {
    requestEvaluations(boards) { if (boards.length) sent.push({ event: 'evaluation_request', boards }); },
    sendEvent(event, boards) { if (boards.length) sent.push({ event, boards }); },
  };
  game.pgnFlipBoard();
  game.pgnFlipBoard();
  assert.equal(sent.length, 0, 'no requests sent when flipping with fully-cached children');
});

test('evaluation re-renders use the flipped board, so scores land on the flipped squares', async () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.pgnCurrentPly = 0;
  const rendered = [];
  game.renderEvaluations = (board) => rendered.push(board);

  game.flipped = true;
  // Every path that re-renders scores without a full pgnRenderCurrentPly: a late evaluation
  // arriving over the websocket, and a wasm worker result.
  game.handleEvaluations([{ board: game.pgnChildrenByPly[0][0], score: 42, source: 'edax', level: 24 }]);
  game._scheduleLocalEvalRender();
  await new Promise((resolve) => setTimeout(resolve, 1)); // let the rAF-batched render run

  const expected = game.pgnBoards[0].rotate(3).toString();
  assert.ok(rendered.length > 0, 'expected at least one re-render');
  for (const board of rendered) {
    assert.equal(board.toString(), expected, 're-render used the unflipped board');
  }
});

test('a click on the flipped board plays the same move as square 63-index', () => {
  const game = buildGame(FORCED_PASS_BOARDS);
  game.pgnCurrentPly = 0;
  const played = firstMoveTo(game.pgnBoards[0], game.pgnBoards[1]); // underlying square of the played move
  assert.ok(played >= 0);

  // Flipped: clicking display cell (63 - played) maps back to `played`, so it advances the line.
  game.flipped = true;
  game.pgnOnSquareClick(63 - played);
  assert.equal(game.pgnIsDiverged(), false, 'clicking the played move on the flipped board did not diverge');
  assert.equal(game.pgnCurrentPly, 1, 'the line advanced, proving the click coordinate was remapped');
});

// Regression: the flip only rotates what is drawn, so every render must position things by the
// *oriented* board (pgnDisplayBoardOriented). Renders triggered by an evaluation arriving
// asynchronously used to go through the unflipped board instead, which put each score on the
// mirrored square — on top of a disc — and left the stale ones behind.

test('scores land on legal squares when an evaluation arrives while flipped', () => {
  const dom = installCellDOM();
  try {
    const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
    game.pgnCurrentPly = 10;
    game.pgnFlipBoard();

    const expected = validMoveIndices(game.pgnDisplayBoardOriented());
    const evaluations = game.pgnDisplayBoardOriented().getChildren().map((child, i) => ({
      board: child.normalize().toString(), score: i - 3, source: 'edax', level: 20,
    }));
    game.handleEvaluations(evaluations);

    assert.deepEqual(dom.scoredIndices(), expected, 'scores sit on the flipped board\'s legal moves');
    assert.deepEqual(dom.scoredIndices().filter((i) => dom.discIndices().includes(i)), [], 'no score on a disc');
  } finally {
    dom.restore();
  }
});

test('scores follow the explored position when clicking around on a flipped board', () => {
  const dom = installCellDOM();
  const game = buildGame(FORCED_PASS_BOARDS, { complete: false });
  try {
    game.pgnCurrentPly = 10;
    game.pgnFlipBoard();

    // Explore two moves deep, evaluating the displayed children at each step the way the server
    // and the local wasm evaluator do while the user clicks around.
    for (let depth = 0; depth < 2; depth++) {
      const moves = validMoveIndices(game.pgnDisplayBoardOriented());
      game.pgnOnSquareClick(moves[moves.length - 1]);

      const expected = validMoveIndices(game.pgnDisplayBoardOriented());
      game.handleEvaluations(game.pgnDisplayBoardOriented().getChildren().map((child, i) => ({
        board: child.normalize().toString(), score: i - 3, source: 'edax', level: 20,
      })));

      assert.ok(game.pgnIsDiverged(), 'clicking a non-PGN move diverges');
      assert.deepEqual(dom.scoredIndices(), expected, `scores match the explored position at depth ${depth}`);
      assert.deepEqual(dom.scoredIndices().filter((i) => dom.discIndices().includes(i)), [], 'no score on a disc');
    }

    // Going back up the explored line drops the scores that no longer belong to any legal move.
    game.pgnGoBack();
    assert.deepEqual(dom.scoredIndices().filter((i) => dom.discIndices().includes(i)), [], 'no score left on a disc');
  } finally {
    game.stopPGNPolling(); // pgnOnSquareClick starts polling; tests never wait on it
    dom.restore();
  }
});

test('a local wasm evaluation rendered a frame later respects the flip', async () => {
  const dom = installCellDOM();
  try {
    const game = buildGame(FORCED_PASS_BOARDS);
    game.pgnCurrentPly = 10;
    game.pgnFlipBoard();

    const expected = validMoveIndices(game.pgnDisplayBoardOriented());
    game._scheduleLocalEvalRender();
    await new Promise((resolve) => setTimeout(resolve, 5)); // let the requestAnimationFrame stub fire

    assert.equal(game._localEvalRenderPending, false, 'the batched render ran');
    assert.deepEqual(dom.scoredIndices(), expected, 'the batched render used the flipped board');
  } finally {
    dom.restore();
  }
});
