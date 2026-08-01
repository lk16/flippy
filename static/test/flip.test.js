// Tests for the F-key board flip on /game's PGN review section: a purely visual 180° rotation
// (rotate(3): square i -> 63-i) that must not change evaluations, the score graph, or which move
// a click plays.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildGame } = require('./harness');
const { FORCED_PASS_BOARDS } = require('./fixtures');

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
