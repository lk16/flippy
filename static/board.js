const BITBOARD_MASK = 0xFFFFFFFFFFFFFFFFn;

// LOCAL_EVAL_LEVELS: incremental depths queueLocalEvaluations searches through, in order, for
// each board the server hasn't evaluated yet. Evaluating at 4 first (sub-millisecond, so every
// child shows a score as good as immediately) and refining through 6, 8, 10 (the server's
// PriorityLevel) and on up lets the UI show a rough score right away and sharpen it in place as
// deeper searches finish, rather than blocking on the deepest result -- which costs seconds per
// board -- before showing anything.
//
// Which of these rungs a given position actually runs depends on how many empty squares it has:
// see localEvalLevelsFor.
const LOCAL_EVAL_LEVELS = [4, 6, 8, 10, 12, 14, 16];

// Local searches are scheduled shallow-first across every queued board at once, by handing
// EdaxEvalWorkerPool.evaluate the search level as its priority: every pending level-4 search runs
// before any level-6, and so on. So the moves on screen all get a number within milliseconds and
// then sharpen together, instead of one board being refined all the way up its ladder (seconds)
// while the move next to it still shows nothing.
//
// LOCAL_EVAL_PREFETCH_PRIORITY is added to that priority for off-screen prefetch work
// (requestGrandchildrenEvaluations). Larger than any LOCAL_EVAL_LEVELS entry, so every visible
// search -- at any level -- goes first.
const LOCAL_EVAL_PREFETCH_PRIORITY = 100;

// localEvalLevelsFor returns the rungs of LOCAL_EVAL_LEVELS worth searching for a position with
// nEmpties empty squares, in order. Edax's level does not mean "search this deep": it means "search
// this deep *unless* few enough squares are left to solve the game outright", and the cutover point
// is per-level (search_global_init, ported in wasm/edax-eval/src/search.rs depth_and_selectivity).
// Whether a rung is worth running follows from which side of its cutover the position sits on:
//
//   - level L <= 10, nEmpties > 2L: fixed-depth midgame search, cost grows with L. Run it.
//   - level L <= 10, nEmpties <= 2L: exact full-width solve. Run it -- and stop, because the score
//     is the game-theoretic result, so every deeper rung would burn the same seconds-to-minutes
//     recomputing a number that cannot change.
//   - level L >= 11, nEmpties > 24 (L <= 12) or > 27 (L >= 13): fixed-depth midgame search with
//     ProbCut. Run it.
//   - level L >= 11, otherwise: an endgame solve over 21+ empties -- minutes in the browser, and
//     selective above 21 empties so not even exact. Stop; level 10 already answered as well as we
//     can afford to.
//
// So an opening position climbs the whole ladder to 16, a midgame one stops where the endgame
// solves start, and an endgame one stops at the first rung that solves it exactly.
function localEvalLevelsFor(nEmpties) {
    const levels = [];
    for (const level of LOCAL_EVAL_LEVELS) {
        if (level <= 10) {
            levels.push(level);
            if (nEmpties <= 2 * level) break;
        } else if (nEmpties > (level <= 12 ? 24 : 27)) {
            levels.push(level);
        } else {
            break;
        }
    }
    return levels;
}

// WebSocketClient batches evaluation lookups over a persistent connection, auto-reconnecting on
// disconnect; requests made before the socket opens are accumulated in pendingBoards and flushed
// together once it connects.
class WebSocketClient {
    constructor(onEvaluations) {
        this.ws = null;
        this.messageId = 1;
        this.onEvaluations = onEvaluations;
        this.pendingBoards = null;
        this.connect();
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

        this.ws.onopen = () => {
            if (this.pendingBoards) {
                this.sendEvent('evaluation_request', this.pendingBoards);
                this.pendingBoards = null;
            }
        };

        this.ws.onmessage = (event) => {
            try {
                const response = JSON.parse(event.data);
                if (response.data && response.data.evaluations) {
                    this.onEvaluations(response.data.evaluations);
                }
            } catch (error) {
                console.error('Error parsing WebSocket message:', error);
            }
        };

        this.ws.onclose = () => {
            setTimeout(() => this.connect(), 1000);
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };
    }

    requestEvaluations(boards) {
        if (boards.length === 0) return;

        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            this.pendingBoards = this.pendingBoards ? [...this.pendingBoards, ...boards] : boards;
            return;
        }

        this.sendEvent('evaluation_request', boards);
    }

    sendEvent(eventName, boards, level = 0) {
        if (!boards.length || !this.ws || this.ws.readyState !== WebSocket.OPEN) return;
        const data = { boards };
        if (level > 0) data.level = level;
        this.ws.send(JSON.stringify({
            id: this.messageId++,
            event: eventName,
            data,
        }));
    }
}

function flipHorizontally(x) {
    const k1 = 0x5555555555555555n;
    const k2 = 0x3333333333333333n;
    const k4 = 0x0F0F0F0F0F0F0F0Fn;

    x = ((x >> 1n) & k1) | ((x & k1) << 1n);
    x = ((x >> 2n) & k2) | ((x & k2) << 2n);
    x = ((x >> 4n) & k4) | ((x & k4) << 4n);
    return x & BITBOARD_MASK;
}

function flipVertically(x) {
    const k1 = 0x00FF00FF00FF00FFn;
    const k2 = 0x0000FFFF0000FFFFn;

    x = ((x >> 8n) & k1) | ((x & k1) << 8n);
    x = ((x >> 16n) & k2) | ((x & k2) << 16n);
    x = (x >> 32n) | (x << 32n);
    return x & BITBOARD_MASK;
}

function flipDiagonally(x) {
    const k1 = 0x5500550055005500n;
    const k2 = 0x3333000033330000n;
    const k4 = 0x0F0F0F0F00000000n;

    let t = k4 & (x ^ (x << 28n));
    x ^= t ^ (t >> 28n);
    t = k2 & (x ^ (x << 14n));
    x ^= t ^ (t >> 14n);
    t = k1 & (x ^ (x << 7n));
    x ^= t ^ (t >> 7n);
    return x & BITBOARD_MASK;
}

function rotateBits(x, rotation) {
    if ((rotation & 1) !== 0) x = flipHorizontally(x);
    if ((rotation & 2) !== 0) x = flipVertically(x);
    if ((rotation & 4) !== 0) x = flipDiagonally(x);
    return x;
}

// OthelloBoard mirrors internal/othello.Board: playerBits/opponentBits are relative to blackTurn.
class OthelloBoard {
    constructor() {
        this.playerBits = 0n;
        this.opponentBits = 0n;
        this.blackTurn = true;

        this.setDisc(3 * 8 + 3, 'white');
        this.setDisc(3 * 8 + 4, 'black');
        this.setDisc(4 * 8 + 3, 'black');
        this.setDisc(4 * 8 + 4, 'white');
    }

    // fromBits builds a board directly from its field values, skipping the
    // constructor's starting-position setup (which callers below would only
    // overwrite anyway).
    static fromBits(playerBits, opponentBits, blackTurn) {
        const board = Object.create(OthelloBoard.prototype);
        board.playerBits = playerBits;
        board.opponentBits = opponentBits;
        board.blackTurn = blackTurn;
        return board;
    }

    clone() {
        return OthelloBoard.fromBits(this.playerBits, this.opponentBits, this.blackTurn);
    }

    getDisc(index) {
        const bit = 1n << BigInt(index);
        if (this.playerBits & bit) {
            return this.blackTurn ? 'black' : 'white';
        }
        if (this.opponentBits & bit) {
            return this.blackTurn ? 'white' : 'black';
        }
        return 'empty';
    }

    setDisc(index, player) {
        const bit = 1n << BigInt(index);
        if (player === 'black') {
            this.playerBits |= bit;
        } else {
            this.opponentBits |= bit;
        }
    }

    countDiscs(color) {
        const bits = (color === 'black') === this.blackTurn ? this.playerBits : this.opponentBits;
        return popcount(bits);
    }

    passMove() {
        this.blackTurn = !this.blackTurn;
        [this.playerBits, this.opponentBits] = [this.opponentBits, this.playerBits];
    }

    hasValidMoves() {
        return [...Array(64).keys()].some((i) => this.isValidMove(i));
    }

    isValidMove(index) {
        if (this.getDisc(index) !== 'empty') {
            return false;
        }
        const directions = [[-1, -1], [-1, 0], [-1, 1], [0, -1], [0, 1], [1, -1], [1, 0], [1, 1]];
        return directions.some(([dx, dy]) => this.getFlippedPerDirection(index, dx, dy) > 0n);
    }

    getValidMoves() {
        let moves = 0n;
        for (let i = 0; i < 64; i++) {
            if (this.isValidMove(i)) moves |= (1n << BigInt(i));
        }
        return moves;
    }

    countMoves() {
        return popcount(this.getValidMoves());
    }

    isGameOver() {
        if (this.hasValidMoves()) return false;
        const clone = this.clone();
        clone.passMove();
        return !clone.hasValidMoves();
    }

    getFlippedPerDirection(index, dx, dy) {
        if (this.getDisc(index) !== 'empty') return 0n;

        const row = index >> 3;
        const col = index & 7;
        const ownColor = this.blackTurn ? 'black' : 'white';

        let flips = 0n;
        let x = row + dx;
        let y = col + dy;

        while (x >= 0 && x < 8 && y >= 0 && y < 8) {
            const currentIndex = x * 8 + y;
            const disc = this.getDisc(currentIndex);

            if (disc === 'empty') return 0n;
            if (disc === ownColor) return flips;

            flips |= (1n << BigInt(currentIndex));
            x += dx;
            y += dy;
        }

        return 0n;
    }

    getFlipped(index) {
        const directions = [[-1, -1], [-1, 0], [-1, 1], [0, -1], [0, 1], [1, -1], [1, 0], [1, 1]];
        let flipped = 0n;
        for (const [dx, dy] of directions) {
            flipped |= this.getFlippedPerDirection(index, dx, dy);
        }
        return flipped;
    }

    doMove(index) {
        if (index < 0 || index >= 64) return null;

        const flipped = this.getFlipped(index);
        if (flipped === 0n) return null;

        const opponentBits = this.playerBits | flipped | (1n << BigInt(index));
        const playerBits = this.opponentBits & ~opponentBits;
        return OthelloBoard.fromBits(playerBits, opponentBits, !this.blackTurn);
    }

    getChildren() {
        return [...Array(64).keys()].map((i) => this.doMove(i)).filter(Boolean);
    }

    rotate(r) {
        return OthelloBoard.fromBits(rotateBits(this.playerBits, r), rotateBits(this.opponentBits, r), this.blackTurn);
    }

    isLessThan(other) {
        if (this.playerBits < other.playerBits) return true;
        return this.playerBits === other.playerBits && this.opponentBits < other.opponentBits;
    }

    // normalize returns the canonical form among the 8 symmetries, matching Go's Board.Normalize.
    normalize() {
        let min = this.clone();
        for (let r = 1; r < 8; r++) {
            const rotated = this.rotate(r);
            if (rotated.isLessThan(min)) min = rotated;
        }
        return min;
    }

    // toString matches Go's Board.String(): 16 hex digits black, 16 hex digits white, then "-b"/"-w".
    toString() {
        const black = this.blackTurn ? this.playerBits : this.opponentBits;
        const white = this.blackTurn ? this.opponentBits : this.playerBits;
        const turnSuffix = this.blackTurn ? '-b' : '-w';
        return black.toString(16).padStart(16, '0') + white.toString(16).padStart(16, '0') + turnSuffix;
    }

    // fromString parses the format produced by toString() / Go's Board.String():
    // 16 hex digits black, 16 hex digits white, "-b" or "-w". Returns null on any parse error.
    static fromString(s) {
        if (typeof s !== 'string' || s.length !== 34) return null;
        const suffix = s.slice(32);
        let blackTurn;
        if (suffix === '-b') blackTurn = true;
        else if (suffix === '-w') blackTurn = false;
        else return null;

        let blackBits, whiteBits;
        try {
            blackBits = BigInt('0x' + s.slice(0, 16));
            whiteBits = BigInt('0x' + s.slice(16, 32));
        } catch (_) {
            return null;
        }

        const playerBits = blackTurn ? blackBits : whiteBits;
        const opponentBits = blackTurn ? whiteBits : blackBits;
        return OthelloBoard.fromBits(playerBits, opponentBits, blackTurn);
    }
}

function popcount(bits) {
    let count = 0n;
    let n = bits;
    while (n > 0n) {
        count += n & 1n;
        n >>= 1n;
    }
    return Number(count);
}

class OthelloGame {
    constructor() {
        // ── Game state ────────────────────────────────────────────────────────
        this.evaluations = new Map(); // normalized board string -> evaluation
        this.wsClient = new WebSocketClient((evals) => this.handleEvaluations(evals));
        this.board = new OthelloBoard();
        this.boardHistory = [];
        this.evalMode = true;
        this.evalPollTimer = null;
        this.evalPollStart = 0;

        // ── PGN state ─────────────────────────────────────────────────────────
        this.pgnState = null;           // null | 'input' | 'graph'
        this.pgnBoards = [];            // OthelloBoard[] parsed from PGN sequence
        this.pgnCurrentPly = 0;
        this.pgnChildrenByPly = [];     // pgnChildrenByPly[i] = normalized child strings for ply i
        this.pgnAllChildStrings = [];   // all unique normalized child strings across all plies
        this.pgnPollTimer = null;
        this.pgnPollStart = 0;
        this.pgnDebounceTimer = null;
        this._graphData = null;
        this._graphLayout = null;
        this._graphClickBound = false;

        // Divergence stack (click-to-explore) while reviewing a PGN line: when non-empty the
        // user has clicked a move that leaves the PGN line; the top of the stack is the board
        // shown, pgnCurrentPly stays frozen at the divergence point, and the score graph keeps
        // reflecting the PGN line.
        this.pgnAlternativeMoves = []; // OthelloBoard[]

        // Board flip (F key), scoped to PGN review: a purely visual 180° rotation. rotate(3)
        // maps every square i to 63-i and leaves normalize() unchanged, so no evaluations or
        // graph data change.
        this.flipped = false;

        // Level-increment tracking, scoped to PGN review.
        this.levelConfig = null;        // fetched from /api/level-config
        this.pendingLevelRequests = new Map(); // board -> highest level we have requested

        // Client-side evaluator for every child the server hasn't answered for yet -- both
        // positions beyond levelConfig.maxSavableDiscs (see internal/book.MaxSavableDiscs), which
        // the server never evaluates at all, and on-book ones whose server evaluation is still
        // being computed. Uses a pool of Web Workers, each running the WASM module off the main
        // thread so evaluations never block the browser UI.
        this.edaxWorkerPool = null;
        this._pendingLocalEvals = new Set(); // boardStrs queued to workers but not yet resolved
        this._localEvalRenderPending = false; // rAF batching flag for worker result renders
        this._localEvalBoardKey = null;  // board whose moves the queued local work belongs to
        this._localEvalGeneration = 0;   // bumped when that board changes; see _syncLocalEvalGeneration

        this.initializeBoard();
        this.initializeButtons();
        this.renderBoard(null, false);
        this.fetchLevelConfig();
        this.initEdaxEval();
    }

    // initEdaxEval creates the worker pool (each worker fetches and initializes the WASM module
    // independently). Positions already on screen that were waiting on it are filled in once all
    // workers report ready. Evaluation results then arrive asynchronously and trigger re-renders
    // via _scheduleLocalEvalRender, so the main thread never blocks waiting for WASM.
    initEdaxEval() {
        const numWorkers = Math.min(4, navigator.hardwareConcurrency || 2);
        this.edaxWorkerPool = new EdaxEvalWorkerPool(
            '/static/wasm/js/edax-eval-worker.js',
            '/static/wasm/dist/edax_eval.wasm',
            '/static/wasm/dist/weights.bin.gz',
            numWorkers,
            // Reserve a worker for the shallowest level, so a move that just appeared on screen
            // gets its first score without waiting behind a deep search already running.
            { fastLaneMaxLevel: LOCAL_EVAL_LEVELS[0] },
        );
        this.edaxWorkerPool.ready()
            .then(() => {
                this.requestMissingEvaluations(this.board);
                this.renderEvaluations(this.pgnState === 'graph' ? this.pgnDisplayBoard() : this.board);
            })
            .catch((error) => {
                console.error('edax-eval: failed to load WASM evaluator; positions beyond the saved book stay unevaluated', error);
            });
    }

    async fetchLevelConfig() {
        try {
            const r = await fetch('/api/level-config');
            if (r.ok) {
                const data = await r.json();
                this.levelConfig = {
                    priorityLevel: data.priority_level,
                    maxSavableDiscs: data.max_savable_discs,
                    targetLevels: data.target_levels.map((t) => ({ maxDiscs: t.max_discs, level: t.level })),
                };
            }
        } catch (_) {}
        // Fallback so the rest of the code always has a config object. Must stay in step with
        // internal/api/level.go: a target above what the server is willing to search is one it
        // never reaches, so isAtTarget would never come true for those boards.
        if (!this.levelConfig) {
            this.levelConfig = {
                priorityLevel: 10,
                maxSavableDiscs: 30,
                targetLevels: [{ maxDiscs: 16, level: 32 }, { maxDiscs: 20, level: 30 }, { maxDiscs: 64, level: 28 }],
            };
        }
    }

    // discCountFromBoardStr counts total discs in a normalized board string (34 chars).
    discCountFromBoardStr(s) {
        const black = BigInt('0x' + s.slice(0, 16));
        const white = BigInt('0x' + s.slice(16, 32));
        return popcount(black) + popcount(white);
    }

    // targetLevelForBoard returns the final target edax level for a board string. Mirrors
    // api.EffectiveTargetLevel: pick the tier the disc count falls in, with boards past
    // maxSavableDiscs treated as if they had exactly that many discs.
    targetLevelForBoard(boardStr) {
        const dc = Math.min(this.discCountFromBoardStr(boardStr), this.levelConfig.maxSavableDiscs);
        const tiers = this.levelConfig.targetLevels;
        const tier = tiers.find((t) => dc <= t.maxDiscs);
        return (tier || tiers[tiers.length - 1]).level;
    }

    // isAtTarget returns true when a board has reached its target evaluation level.
    isAtTarget(boardStr) {
        const e = this.evaluations.get(boardStr);
        if (!e) return false;
        if (e.source === 'minimax' || e.source === 'final') return true;
        return (e.level || 0) >= this.targetLevelForBoard(boardStr);
    }

    // shouldUpdateEval returns true when incoming should replace existing in the evaluations map.
    // Minimax/final results are never downgraded to edax; among edax results, higher level wins.
    // 'wasm' (queueLocalEvaluations' client-side stand-in) is the lowest priority: it only ever
    // fills a blank, and any server-sourced result always supersedes it -- which is the normal
    // course of events for an on-book position, whose wasm score is only shown until the server
    // finishes analyzing it.
    shouldUpdateEval(existing, incoming) {
        if (!existing) return true;
        if (incoming.source === 'wasm') return false;
        if (existing.source === 'wasm') return true;
        const existingFinal = existing.source === 'minimax' || existing.source === 'final';
        if (existingFinal && incoming.source === 'edax') return false;
        if (existing.source === 'edax' && incoming.source === 'edax') {
            return (incoming.level || 0) > (existing.level || 0);
        }
        return true; // incoming is minimax/final, always wins over edax
    }

    // ── Initialization ────────────────────────────────────────────────────────

    initializeBoard() {
        const boardElement = document.getElementById('board');
        boardElement.innerHTML = '';

        boardElement.addEventListener('contextmenu', (e) => {
            e.preventDefault();
            this.undoMove();
        });

        for (let index = 0; index < 64; index++) {
            const cell = document.createElement('div');
            cell.className = 'cell';
            cell.dataset.index = index;
            cell.addEventListener('click', () => this.onCellClick(index));
            boardElement.appendChild(cell);
        }
    }

    initializeButtons() {
        document.getElementById('undo-button').addEventListener('click', () => this.undoMove());
        document.getElementById('new-game-button').addEventListener('click', () => this.newGame());
        document.getElementById('eval-mode-button').addEventListener('click', () => this.toggleEvalMode());
        document.getElementById('load-pgn-button').addEventListener('click', () => this.togglePGNSection());
        document.getElementById('pgn-input').addEventListener('input', () => this.onPGNInput());

        document.addEventListener('keydown', (e) => {
            if (this.pgnState !== 'graph') return;
            if (e.key === 'ArrowLeft') this.pgnGoBack();
            if (e.key === 'ArrowRight') this.pgnGoForward();
            if (e.key === 'f' || e.key === 'F') this.pgnFlipBoard();
        });
    }

    // ── Game mode ─────────────────────────────────────────────────────────────

    toggleEvalMode() {
        this.evalMode = !this.evalMode;
        const button = document.getElementById('eval-mode-button');
        button.textContent = this.evalMode ? 'Hide evals' : 'Show evals';

        const currentBoard = this.pgnState === 'graph' ? this.pgnBoards[this.pgnCurrentPly] : this.board;

        if (this.evalMode) {
            this.requestMissingEvaluations(currentBoard);
            this.requestGrandchildrenEvaluations(currentBoard);
            this.renderEvaluations(currentBoard);
        } else {
            document.querySelectorAll('.cell .score-display').forEach((el) => el.remove());
            document.querySelectorAll('.best-move-circle').forEach((el) => el.remove());
            const validMoves = currentBoard.getValidMoves();
            document.querySelectorAll('.cell').forEach((cell) => {
                const index = parseInt(cell.dataset.index, 10);
                cell.classList.toggle('valid-move', ((1n << BigInt(index)) & validMoves) !== 0n);
            });
        }
    }

    newGame() {
        this.setPGNState(null);
        this.board = new OthelloBoard();
        this.boardHistory = [];
        this.renderBoard(null, false);
    }

    onCellClick(index) {
        if (this.pgnState === 'graph') {
            this.pgnOnSquareClick(index);
            return;
        }

        const child = this.board.doMove(index);
        if (!child) return;

        const previousBoard = this.board.clone();
        this.boardHistory.push(previousBoard);
        this.board = child;

        // Resolve up to two forced passes before handing control back to the player.
        for (let i = 0; i < 2 && !this.board.hasValidMoves(); i++) {
            this.boardHistory.push(this.board.clone());
            this.board.passMove();
        }

        this.renderBoard(previousBoard, true);
    }

    undoMove() {
        if (this.pgnState === 'graph') {
            // While reviewing a PGN line, "undo" (right-click or the Undo button) mirrors the
            // left arrow: pop one explored move while diverged, otherwise step back a ply.
            this.pgnGoBack();
            return;
        }
        if (this.boardHistory.length === 0) return;

        const previousState = this.board.clone();
        while (this.boardHistory.length > 0) {
            this.board = this.boardHistory.pop();
            if (this.board.hasValidMoves()) break;
        }

        this.renderBoard(previousState, true);
    }

    renderBoard(previousBoard, animate) {
        document.querySelectorAll('.cell .score-display').forEach((el) => el.remove());
        document.querySelectorAll('.best-move-circle').forEach((el) => el.remove());
        document.querySelectorAll('.cell.next-move-played').forEach((el) => el.classList.remove('next-move-played'));

        document.querySelectorAll('.cell').forEach((cell) => {
            const index = parseInt(cell.dataset.index, 10);
            const disc = this.board.getDisc(index);
            const existingPiece = cell.querySelector('.piece');

            if (existingPiece) {
                if (disc === 'empty') {
                    if (animate) {
                        existingPiece.style.opacity = '0';
                        setTimeout(() => existingPiece.parentNode === cell && cell.removeChild(existingPiece), 300);
                    } else {
                        cell.removeChild(existingPiece);
                    }
                } else {
                    existingPiece.classList.remove('black', 'white');
                    existingPiece.classList.add(disc);
                }
            } else if (disc !== 'empty') {
                const piece = document.createElement('div');
                piece.className = `piece ${disc}`;
                if (animate) {
                    piece.style.opacity = '0';
                    cell.appendChild(piece);
                    piece.offsetHeight; // force reflow before fading in
                    piece.style.opacity = '1';
                } else {
                    cell.appendChild(piece);
                }
            }
        });

        this.updateValidMoves(this.board);
        this.updateScore(this.board);
        this.updateGameStatus(this.board);
        this.renderEvaluations(this.board);
    }

    updateValidMoves(board) {
        const boardElement = document.getElementById('board');
        boardElement.classList.toggle('black-turn', board.blackTurn);
        boardElement.classList.toggle('white-turn', !board.blackTurn);
        boardElement.classList.remove('exploring'); // only the PGN review board can be "exploring"

        const validMoves = board.getValidMoves();
        document.querySelectorAll('.cell').forEach((cell) => {
            const index = parseInt(cell.dataset.index, 10);
            cell.classList.toggle('valid-move', ((1n << BigInt(index)) & validMoves) !== 0n);
        });

        this.requestMissingEvaluations(board);
        this.requestGrandchildrenEvaluations(board);
        if (this.hasUnresolvedEvaluations(board)) {
            this.startEvalPolling();
        } else {
            this.stopEvalPolling();
        }
    }

    // needsServerEvaluation reports whether boardStr still wants an evaluation from the server. A
    // wasm score counts as missing here, not as an answer: it is a local stand-in shown while the
    // server's deeper, book-quality evaluation is still being computed, so it must not stop
    // requestMissingEvaluations from asking or startEvalPolling from waiting for the real one.
    needsServerEvaluation(boardStr) {
        const e = this.evaluations.get(boardStr);
        return !e || e.source === 'wasm';
    }

    // hasUnresolvedEvaluations reports whether board's children or grandchildren are missing an
    // on-book evaluation -- i.e. one requestServerAnalysis can ask the server to compute. Off-book
    // boards are excluded: those are handled by queueLocalEvaluations' wasm chain, which needs no
    // polling since each worker result drives its own re-render.
    hasUnresolvedEvaluations(board) {
        const seen = new Set();
        for (const child of board.getChildren()) {
            seen.add(child.normalize().toString());
            for (const grandchild of child.getChildren()) seen.add(grandchild.normalize().toString());
        }
        const missing = [...seen].filter((b) => this.needsServerEvaluation(b));
        const [onBook] = this.splitOffBook(missing);
        return onBook.length > 0;
    }

    // startEvalPolling retries requestMissingEvaluations/requestGrandchildrenEvaluations until every
    // on-book evaluation around the current board has arrived. analyze_request only enqueues a
    // background job and returns whatever's already computed (normally nothing yet for a position
    // outside the pre-explored book), so without this the score would never appear until the next
    // move happens to re-trigger a request. Mirrors PGN mode's startPGNPolling.
    startEvalPolling() {
        if (this.evalPollTimer) return;
        this.evalPollStart = Date.now();
        const INTERVAL = 1750;
        const TIMEOUT = 90000;
        this.evalPollTimer = setInterval(() => {
            if (this.pgnState === 'graph' || !this.evalMode || !this.hasUnresolvedEvaluations(this.board)) {
                this.stopEvalPolling();
                return;
            }
            if (Date.now() - this.evalPollStart > TIMEOUT) {
                this.stopEvalPolling();
                return;
            }
            this.requestMissingEvaluations(this.board);
            this.requestGrandchildrenEvaluations(this.board);
        }, INTERVAL);
    }

    stopEvalPolling() {
        if (this.evalPollTimer) {
            clearInterval(this.evalPollTimer);
            this.evalPollTimer = null;
        }
    }

    // splitOffBook partitions boardStrs into [onBook, offBook] for *server* requests: offBook holds positions beyond
    // levelConfig.maxSavableDiscs, which the server never persists or evaluates (see
    // internal/book.MaxSavableDiscs) -- those must be evaluated locally instead of requested over
    // the websocket. Returns [boardStrs, []] unchanged while levelConfig hasn't loaded yet, so
    // callers fall back to the original all-goes-to-server behavior until it has.
    splitOffBook(boardStrs) {
        if (!this.levelConfig) return [boardStrs, []];
        const onBook = [];
        const offBook = [];
        for (const b of boardStrs) {
            const bucket = this.discCountFromBoardStr(b) > this.levelConfig.maxSavableDiscs ? offBook : onBook;
            bucket.push(b);
        }
        return [onBook, offBook];
    }

    // _syncLocalEvalGeneration notices that the board whose moves we are evaluating locally has
    // changed -- the user played, undid, or clicked to another position -- and abandons the
    // previous board's queued wasm work. Without this, navigating leaves up to a few hundred
    // queued searches for boards nobody is looking at, and the new position's deeper refinements
    // queue up behind all of them.
    //
    // Queued-but-not-started searches are dropped by the pool; searches already running finish and
    // store their result (it is still a correct evaluation, and _runLocalEvalLevels' write only
    // ever deepens what is cached), they just don't continue to the next level.
    _syncLocalEvalGeneration(board) {
        const key = board.normalize().toString();
        if (key === this._localEvalBoardKey) return;
        this._localEvalBoardKey = key;
        this._localEvalGeneration++;
        this._pendingLocalEvals.clear();
        if (this.edaxWorkerPool) {
            this.edaxWorkerPool.cancelQueued((tag) => tag !== this._localEvalGeneration);
        }
    }

    // queueLocalEvaluations sends boardStrs to the worker pool for asynchronous evaluation via
    // the client-side WASM evaluator (wasm/edax-eval), one independent LOCAL_EVAL_LEVELS chain
    // per board (see _runLocalEvalLevels). Returns immediately -- results arrive via worker
    // messages and trigger a batched re-render through _scheduleLocalEvalRender. A no-op until
    // initEdaxEval() creates the pool.
    //
    // Skips boards that are already queued, that the server has answered for, and that a local
    // chain already took to the deepest level worth searching for that position (localEvalLevelsFor
    // -- which rungs those are depends on the position's empty-square count). A board left part-way
    // up its ladder -- by _syncLocalEvalGeneration cancelling it as prefetch, say, before the user
    // navigated to it -- resumes at the first level deeper than what it has, rather than being
    // written off.
    //
    // `prefetch` marks work for boards that are not on screen (grandchildren), which the pool then
    // runs only when nothing visible is waiting; see LOCAL_EVAL_PREFETCH_PRIORITY.
    queueLocalEvaluations(boardStrs, { prefetch = false } = {}) {
        if (!this.edaxWorkerPool) return;
        const generation = this._localEvalGeneration;
        for (const boardStr of boardStrs) {
            if (this._pendingLocalEvals.has(boardStr)) continue;
            const levels = localEvalLevelsFor(64 - this.discCountFromBoardStr(boardStr));
            const deepestLevel = levels[levels.length - 1];
            const existing = this.evaluations.get(boardStr);
            if (existing && (existing.source !== 'wasm' || (existing.level || 0) >= deepestLevel)) continue;
            const levelIndex = levels.findIndex((l) => l > (existing ? existing.level || 0 : 0));
            const board = OthelloBoard.fromString(boardStr);
            if (!board) continue;
            this._pendingLocalEvals.add(boardStr);
            this._runLocalEvalLevels(boardStr, board, levels, levelIndex, { prefetch, generation });
        }
    }

    // _runLocalEvalLevels evaluates boardStr at levels[levelIndex], stores the result
    // and schedules a render, then recurses into the next (deeper) level -- so the displayed
    // score starts shallow and refines in place rather than blocking on the deepest level. Since
    // every board's searches are queued with the level as their priority, this comes out as one
    // shallow-first sweep across all boards, not a race between per-board chains.
    //
    // Stops early if a server-sourced ('edax'/'minimax'/'final') evaluation supersedes this board
    // while the chain is running -- no point spending more worker time refining a wasm score once
    // a real one has arrived -- or if `generation` is stale, i.e. the user has moved on from the
    // position this chain was queued for.
    _runLocalEvalLevels(boardStr, board, levels, levelIndex, { prefetch, generation }) {
        // Releases boardStr's pending slot, unless a newer generation has re-queued it: then the
        // slot belongs to that chain, not this (stale) one.
        const clearPending = () => {
            if (generation === this._localEvalGeneration) this._pendingLocalEvals.delete(boardStr);
        };

        const existing = this.evaluations.get(boardStr);
        if (existing && existing.source !== 'wasm') {
            clearPending();
            return;
        }
        const level = levels[levelIndex];
        const priority = level + (prefetch ? LOCAL_EVAL_PREFETCH_PRIORITY : 0);
        this.edaxWorkerPool.evaluate(board.playerBits, board.opponentBits, level, { priority, tag: generation })
            .then((score) => {
                const current = this.evaluations.get(boardStr);
                if (current && current.source !== 'wasm') {
                    clearPending();
                    return;
                }
                // Only ever deepen: a search that was already running when the position changed
                // can land after a fresher chain has taken the same board further.
                if (!current || (current.level || 0) < level) {
                    this.evaluations.set(boardStr, { board: boardStr, score, level, source: 'wasm' });
                    this._scheduleLocalEvalRender();
                }

                const nextIndex = levelIndex + 1;
                if (nextIndex < levels.length && generation === this._localEvalGeneration) {
                    this._runLocalEvalLevels(boardStr, board, levels, nextIndex, { prefetch, generation });
                } else {
                    clearPending();
                }
            })
            .catch(() => {
                clearPending();
            });
    }

    // _scheduleLocalEvalRender batches worker result renders: at most one re-render per animation
    // frame, even when many evaluations complete in rapid succession.
    _scheduleLocalEvalRender() {
        if (this._localEvalRenderPending) return;
        this._localEvalRenderPending = true;
        requestAnimationFrame(() => {
            this._localEvalRenderPending = false;
            this.renderEvaluations(this.pgnState === 'graph' ? this.pgnDisplayBoard() : this.board);
        });
    }

    requestMissingEvaluations(board) {
        if (!this.evalMode) return;
        this._syncLocalEvalGeneration(board);
        const children = [...new Set(board.getChildren().map((child) => child.normalize().toString()))];

        const [onBook] = this.splitOffBook(children.filter((b) => this.needsServerEvaluation(b)));
        this.requestServerAnalysis(onBook);

        // Every child the server hasn't answered for goes to the local wasm chain, on-book or
        // not: an on-book position the book hasn't explored yet takes the server seconds to
        // minutes (analyze_request only enqueues a job), and a level-4 local search costs well
        // under a millisecond, so there is no reason to show nothing in the meantime.
        // queueLocalEvaluations skips whatever already has an evaluation or is already queued.
        this.queueLocalEvaluations(children);
    }

    // Prefetch evaluations for grandchildren so they are cached before the user clicks a move.
    //
    // Unlike requestMissingEvaluations, on-book grandchildren are not handed to the wasm chain:
    // there are an order of magnitude more of them than children and none of them is on screen,
    // while the server can answer for them in one batched request. Off-book grandchildren still go
    // local, since for those the server is not an option at all -- queued as prefetch, so the
    // ~100 invisible searches never delay a visible one. Whichever grandchildren the user actually
    // navigates to become children, and requestMissingEvaluations re-queues them at full priority,
    // resuming from whatever level the prefetch reached.
    requestGrandchildrenEvaluations(board) {
        if (!this.evalMode) return;
        const seen = new Set();
        const missing = [];
        for (const child of board.getChildren()) {
            for (const grandchild of child.getChildren()) {
                const key = grandchild.normalize().toString();
                if (!seen.has(key) && this.needsServerEvaluation(key)) {
                    seen.add(key);
                    missing.push(key);
                }
            }
        }
        const [onBook, offBook] = this.splitOffBook(missing);
        this.requestServerAnalysis(onBook);
        this.queueLocalEvaluations(offBook, { prefetch: true });
    }

    handleEvaluations(evaluations) {
        let changed = false;
        for (const evaluation of evaluations) {
            const existing = this.evaluations.get(evaluation.board);
            if (this.shouldUpdateEval(existing, evaluation)) {
                this.evaluations.set(evaluation.board, evaluation);
                changed = true;
            }
        }
        if (!changed) return;

        if (this.pgnState === 'graph') {
            this.pgnRenderGraph();
            this.pgnUpdateGraphStatus();
            this.pgnRequestLevelUps();
            if (this.evalMode) {
                this.renderEvaluations(this.pgnDisplayBoard());
            }
        } else {
            this.renderEvaluations(this.board);
        }
    }

    // pgnRequestLevelUps checks every board that has an evaluation below its target and sends
    // batched analyze_requests (grouped by next level) for those not yet re-requested at that level.
    pgnRequestLevelUps() {
        if (!this.wsClient || !this.levelConfig) return;

        const byLevel = new Map(); // nextLevel -> [boardStr, ...]

        for (const boardStr of this.pgnAllChildStrings) {
            const e = this.evaluations.get(boardStr);
            if (!e) continue; // no evaluation yet — will be picked up later
            if (e.source === 'minimax' || e.source === 'final') continue; // always sufficient
            const target = this.targetLevelForBoard(boardStr);
            const current = e.level || 0;
            if (current >= target) continue; // already at target

            // Never past the target: the server clamps to it anyway (handleAnalyzeRequest), so
            // asking for more would leave pendingLevelRequests -- and the level the status line
            // reports -- claiming a search deeper than any that is actually running.
            const nextLevel = Math.min(current + 2, target);
            const alreadyRequested = (this.pendingLevelRequests.get(boardStr) || 0) >= nextLevel;
            if (alreadyRequested) continue;

            this.pendingLevelRequests.set(boardStr, nextLevel);
            if (!byLevel.has(nextLevel)) byLevel.set(nextLevel, []);
            byLevel.get(nextLevel).push(boardStr);
        }

        for (const [level, boards] of byLevel) {
            this.wsClient.sendEvent('analyze_request', boards, level);
        }
    }

    // renderEvaluations shows each legal move's score (negation of its child's stored score).
    renderEvaluations(board) {
        if (!this.evalMode) return;
        let bestScore = -Infinity;
        const moveEvaluations = new Map();
        let haveAllEvaluations = true;

        for (let index = 0; index < 64; index++) {
            const child = board.doMove(index);
            if (!child) continue;

            const evaluation = this.evaluations.get(child.normalize().toString());
            if (!evaluation) {
                haveAllEvaluations = false;
                continue;
            }

            const score = -evaluation.score;
            bestScore = Math.max(bestScore, score);
            moveEvaluations.set(index, score);
        }

        document.querySelectorAll('.cell').forEach((cell) => {
            const index = parseInt(cell.dataset.index, 10);

            const existingCircle = cell.querySelector('.best-move-circle');
            if (existingCircle) cell.removeChild(existingCircle);

            if (!moveEvaluations.has(index)) return;

            const score = moveEvaluations.get(index);
            cell.classList.remove('valid-move');

            let scoreDisplay = cell.querySelector('.score-display');
            if (!scoreDisplay) {
                scoreDisplay = document.createElement('div');
                scoreDisplay.className = 'score-display';
                cell.appendChild(scoreDisplay);
            }
            scoreDisplay.textContent = score > 0 ? `+${score}` : `${score}`;

            if (haveAllEvaluations && score === bestScore) {
                const circle = document.createElement('div');
                circle.className = 'best-move-circle';
                cell.appendChild(circle);
            }
        });
    }

    updateScore(board) {
        document.getElementById('black-score').textContent = board.countDiscs('black');
        document.getElementById('white-score').textContent = board.countDiscs('white');
    }

    updateGameStatus(board) {
        const blackCount = board.countDiscs('black');
        const whiteCount = board.countDiscs('white');
        const statusElement = document.getElementById('game-status');

        if (board.isGameOver()) {
            if (blackCount > whiteCount) {
                statusElement.textContent = 'Game Over - Black Wins!';
            } else if (whiteCount > blackCount) {
                statusElement.textContent = 'Game Over - White Wins!';
            } else {
                statusElement.textContent = 'Game Over - Draw!';
            }
            return;
        }

        if (!board.hasValidMoves()) {
            statusElement.textContent = `${board.blackTurn ? 'Black' : 'White'} must pass`;
            return;
        }

        const validMoves = board.countMoves();
        const currentPlayer = board.blackTurn ? 'Black' : 'White';
        statusElement.textContent = `${currentPlayer} has ${validMoves} move${validMoves === 1 ? '' : 's'}`;
    }

    // ── PGN section ───────────────────────────────────────────────────────────

    togglePGNSection() {
        if (this.pgnState === null) {
            this.setPGNState('input');
        } else if (this.pgnState === 'input') {
            this.setPGNState(null);
        } else {
            // graph → show textarea with the previously loaded PGN for re-editing
            this.setPGNState('input');
        }
    }

    setPGNState(state) {
        const prev = this.pgnState;
        this.pgnState = state;
        this.stopEvalPolling(); // renderBoard restarts it if state returns to null and it's still needed

        const section = document.getElementById('pgn-section');
        const textarea = document.getElementById('pgn-input');
        const graphSection = document.getElementById('eval-graph-section');
        const errorEl = document.getElementById('pgn-error');

        section.style.display = state === null ? 'none' : '';
        textarea.style.display = state === 'input' ? '' : 'none';
        graphSection.style.display = state === 'graph' ? '' : 'none';
        errorEl.style.display = state === 'graph' ? 'none' : '';
        errorEl.textContent = '';

        if (state === null) {
            this.stopPGNPolling();
            this.pgnBoards = [];
            this.pgnCurrentPly = 0;
        }

        // Restore game board display whenever leaving graph mode
        if (state === null || (state === 'input' && prev === 'graph')) {
            this.renderBoard(null, false);
        }

        // Showing the textarea (only reached via the load-PGN button) — focus it so the
        // user can paste immediately without an extra click. Not focused when the button
        // closes the section (state null) or when the graph is shown.
        if (state === 'input') {
            textarea.focus();
        }
    }

    onPGNInput() {
        clearTimeout(this.pgnDebounceTimer);
        this.pgnDebounceTimer = setTimeout(() => this.analyzePGN(), 600);
    }

    async analyzePGN() {
        const pgn = document.getElementById('pgn-input').value.trim();
        const errorEl = document.getElementById('pgn-error');
        errorEl.textContent = '';

        if (!pgn) return;

        // Ensure level config is ready before we start.
        if (!this.levelConfig) await this.fetchLevelConfig();

        let response;
        try {
            response = await fetch('/api/pgn', { method: 'POST', body: pgn });
        } catch (_) {
            errorEl.textContent = 'Network error — is the server running?';
            return;
        }

        if (!response.ok) {
            const data = await response.json().catch(() => ({}));
            errorEl.textContent = data.error || `Parse error (HTTP ${response.status})`;
            return;
        }

        const { boards: rawStrings } = await response.json();

        this.pgnBoards = rawStrings.map((s) => OthelloBoard.fromString(s)).filter(Boolean);
        this.pgnCurrentPly = 0;
        this.pgnAlternativeMoves = [];
        this.flipped = false;
        this.pendingLevelRequests = new Map();

        this.pgnBuildChildSets();
        this.stopPGNPolling();

        this.setPGNState('graph');
        this.pgnRenderCurrentPly();

        // Give the socket a tick to open (or it may already be open from a previous run).
        setTimeout(() => this.pgnSendRequests(), 50);
        this.startPGNPolling();
    }

    // ── PGN child sets ────────────────────────────────────────────────────────

    pgnBuildChildSets() {
        this.pgnChildrenByPly = [];
        const seen = new Set();

        for (let i = 0; i < this.pgnBoards.length; i++) {
            const board = this.pgnBoards[i];
            if (!board.hasValidMoves()) {
                this.pgnChildrenByPly.push([]);
                continue;
            }
            const children = board.getChildren().map((c) => c.normalize().toString());
            this.pgnChildrenByPly.push(children);
            for (const s of children) seen.add(s);
        }

        this.pgnAllChildStrings = [...seen];
    }

    pgnUnresolved() {
        return this.pgnAllChildStrings.filter((s) => !this.isAtTarget(s));
    }

    // pgnExploreTargets returns the diverged board's own normalized string plus its children —
    // the boards pgnRequestDivergedEvals asked the backend to analyze — or [] when not diverged.
    // Kept separate from pgnAllChildStrings (the fixed PGN-line set) since the explored subtree
    // changes with every click.
    pgnExploreTargets() {
        if (!this.pgnIsDiverged()) return [];
        const board = this.pgnDisplayBoard();
        const targets = new Set([board.normalize().toString()]);
        if (board.hasValidMoves()) {
            for (const c of board.getChildren()) targets.add(c.normalize().toString());
        }
        return [...targets];
    }

    pgnSendRequests() {
        const all = this.pgnAllChildStrings;
        if (!all.length) return;

        const startLevel = this.levelConfig.priorityLevel;
        for (const s of all) this.pendingLevelRequests.set(s, startLevel);

        // evaluation_request uses the buffering path in WebSocketClient; analyze_request is best-effort.
        this.wsClient.requestEvaluations(all);
        this.wsClient.sendEvent('analyze_request', all, startLevel);
    }

    // requestServerAnalysis sends both request kinds (evaluation_request + analyze_request) at the
    // priority level for boards not yet tracked in pendingLevelRequests -- asking the server to
    // actually compute a board the first time it's needed, not just checking whatever it already
    // has saved. Used by PGN's divergence exploration (pgnRequestDivergedEvals) and normal-mode
    // play (requestMissingEvaluations/requestGrandchildrenEvaluations); pgnSendRequests's initial
    // line-wide batch duplicates this instead of calling it, since it always (re)stamps the
    // priority level rather than skipping boards already tracked.
    requestServerAnalysis(list) {
        if (!this.wsClient || !this.levelConfig) return;
        const startLevel = this.levelConfig.priorityLevel;
        for (const s of list) {
            if (!this.pendingLevelRequests.has(s)) this.pendingLevelRequests.set(s, startLevel);
        }
        this.wsClient.requestEvaluations(list);
        this.wsClient.sendEvent('analyze_request', list, startLevel);
    }

    startPGNPolling() {
        this.pgnPollStart = Date.now();
        const INTERVAL = 1750;
        const TIMEOUT = 90000;

        this.pgnPollTimer = setInterval(() => {
            const unresolved = this.pgnUnresolved();
            // Also keep polling the currently explored (diverged) subtree — it isn't part of
            // pgnAllChildStrings, so without this its evaluations would only ever be requested
            // once and a slow edax search would never get picked up.
            const exploring = this.pgnExploreTargets().filter((s) => !this.evaluations.has(s));
            if (!unresolved.length && !exploring.length) {
                this.stopPGNPolling();
                this.pgnUpdateGraphStatus();
                return;
            }
            if (Date.now() - this.pgnPollStart > TIMEOUT) {
                this.stopPGNPolling();
                const statusEl = document.getElementById('graph-status');
                if (statusEl) statusEl.textContent = 'Some positions are still computing. Reload the page to check again.';
                return;
            }
            this.wsClient.requestEvaluations([...new Set([...unresolved, ...exploring])]);
        }, INTERVAL);
    }

    stopPGNPolling() {
        if (this.pgnPollTimer) {
            clearInterval(this.pgnPollTimer);
            this.pgnPollTimer = null;
        }
    }

    pgnUpdateGraphStatus() {
        const total = this.pgnAllChildStrings.length;
        const unresolved = this.pgnUnresolved();
        const done = total - unresolved.length;
        const statusEl = document.getElementById('graph-status');
        if (!statusEl) return;

        if (done >= total) {
            statusEl.textContent = 'Analysis complete.';
            return;
        }

        // Boards ramp up their search level together in +2 rounds (see pgnRequestLevelUps), so
        // the lowest currently-requested level is a fair read of "how deep the search is right
        // now" -- but only across the boards still being searched. Boards already at target keep
        // their last requested level in pendingLevelRequests forever, so counting them in pinned
        // the reported level at whatever the first board finished at (usually priorityLevel) and
        // it never moved again.
        const levels = unresolved.map((s) => this.pendingLevelRequests.get(s) || this.levelConfig.priorityLevel);
        const currentLevel = Math.min(...levels);
        statusEl.textContent = `Searching at level ${currentLevel} — ${done} / ${total} boards evaluated…`;
    }

    // ── PGN board display ─────────────────────────────────────────────────────

    pgnGoTo(ply) {
        if (ply < 0 || ply >= this.pgnBoards.length) return;
        this.pgnCurrentPly = ply;
        this.pgnRenderCurrentPly();
        // Re-render the graph so its highlighted current-ply dot tracks navigation.
        // Without this, the highlight is only redrawn when evaluations change or the
        // graph is clicked, so arrow-key navigation (which skips forced-pass plies)
        // would leave the marker frozen at whatever ply was last rendered.
        this.pgnRenderGraph();
    }

    // pgnIsForcedPass reports whether the board at ply has no legal move while the game is not
    // over — i.e. the player to move must pass but the opponent can still move. The final
    // position, where neither player can move, is game over and is not a forced pass.
    pgnIsForcedPass(ply) {
        const b = this.pgnBoards[ply];
        return !!b && !b.hasValidMoves() && !b.isGameOver();
    }

    // pgnStepPly moves the current ply by delta (+1 / -1), skipping forced-pass positions so
    // arrow-key navigation always lands where a player actually had a move to make. The final
    // game-over position is not skipped. If skipping runs off either end, we stay put.
    pgnStepPly(delta) {
        let ply = this.pgnCurrentPly + delta;
        while (ply >= 0 && ply < this.pgnBoards.length && this.pgnIsForcedPass(ply)) {
            ply += delta;
        }
        if (ply < 0 || ply >= this.pgnBoards.length) return;
        this.pgnGoTo(ply);
    }

    // ── Divergence (click-to-explore) ────────────────────────────────────────
    // Mirrors old/python/flippy/src/flippy/mode/pgn.py PGNMode's alternative_moves stack.

    // pgnIsDiverged reports whether the user is exploring off the PGN line.
    pgnIsDiverged() {
        return this.pgnAlternativeMoves.length > 0;
    }

    // pgnDisplayBoard is the board currently shown: the top of the divergence stack while
    // diverged, otherwise the PGN board at pgnCurrentPly (mirrors pgn.py get_board).
    pgnDisplayBoard() {
        if (this.pgnIsDiverged()) return this.pgnAlternativeMoves[this.pgnAlternativeMoves.length - 1];
        return this.pgnBoards[this.pgnCurrentPly];
    }

    // pgnOnSquareClick plays the clicked move on the displayed board (mirrors pgn.py on_move).
    // Clicking the move actually played in the PGN advances the line like arrow-right; any other
    // legal move diverges by pushing the resulting board onto the divergence stack.
    pgnOnSquareClick(index) {
        const board = this.pgnDisplayBoard();
        if (!board) return;

        // When the board is flipped, the clicked display cell maps to square 63-index.
        const square = this.flipped ? 63 - index : index;
        const child = board.doMove(square);
        if (!child) return; // not a legal move

        const nextBoard = this.pgnCurrentPly + 1 < this.pgnBoards.length ? this.pgnBoards[this.pgnCurrentPly + 1] : null;
        if (!this.pgnIsDiverged() && nextBoard && child.toString() === nextBoard.toString()) {
            // The clicked square is the move actually played in the game — not a divergence.
            // Advance like arrow-right (pgnStepPly also skips the forced pass it may create).
            this.pgnStepPly(1);
            return;
        }

        // Diverge: push the resulting board.
        let top = child;
        this.pgnAlternativeMoves.push(top);

        // Forced pass: the move left the opponent with no legal move but the game isn't over —
        // auto-advance through the pass so the user isn't stuck on an unplayable board
        // (mirrors pgn.py on_move's pass handling).
        if (!top.hasValidMoves() && !top.isGameOver()) {
            top = top.clone();
            top.passMove();
            this.pgnAlternativeMoves[this.pgnAlternativeMoves.length - 1] = top;
        }

        this.pgnRequestDivergedEvals();
        // Re-render the board, but NOT the graph: it keeps reflecting the PGN line.
        this.pgnRenderCurrentPly();
    }

    // pgnGoBack is the left-arrow action: while diverged it pops one explored move (mirrors
    // pgn.py show_prev_position), otherwise it steps to the previous PGN ply.
    pgnGoBack() {
        if (this.pgnIsDiverged()) {
            this.pgnAlternativeMoves.pop();
            this.pgnRenderCurrentPly(); // graph unchanged; pgnCurrentPly is still the divergence ply
            return;
        }
        this.pgnStepPly(-1);
    }

    // pgnGoForward is the right-arrow action. While diverged it is a no-op (mirrors pgn.py
    // show_next_position returning early when alternative_moves is non-empty); otherwise it
    // steps to the next PGN ply.
    pgnGoForward() {
        if (this.pgnIsDiverged()) return;
        this.pgnStepPly(1);
    }

    // pgnFlipBoard toggles a purely visual 180° rotation of the board (mirrors pgn.py flip_board's
    // 63 - move point rotation). It only affects rendering and click coordinates — evaluations,
    // pgnChildrenByPly and the score graph are unaffected because rotate(3) leaves normalize()
    // unchanged. The graph does not change, so only the board is re-rendered.
    pgnFlipBoard() {
        this.flipped = !this.flipped;
        this.pgnRenderCurrentPly();
    }

    // pgnRequestDivergedEvals asks the backend to evaluate the current diverged board and its
    // children, reusing the same request pipeline as the PGN line so the explored position's
    // on-board move overlay can be shown. It never touches the PGN tracking state. Also (re)starts
    // polling: the initial analyze_request only returns whatever's already available, and the
    // explored subtree needs the same retry-until-resolved treatment as the PGN line so a slow
    // edax search still gets picked up once it completes.
    pgnRequestDivergedEvals() {
        this.requestServerAnalysis(this.pgnExploreTargets());
        if (!this.pgnPollTimer) this.startPGNPolling();
    }

    pgnRenderCurrentPly() {
        const underlying = this.pgnDisplayBoard();
        if (!underlying) return;
        // Apply the visual flip (rotate(3): square i -> 63-i). Disc counts, turn, moves and
        // game-over state are all rotation-invariant, so everything below can use `board`.
        const board = this.flipped ? underlying.rotate(3) : underlying;

        document.querySelectorAll('.cell .score-display').forEach((el) => el.remove());
        document.querySelectorAll('.best-move-circle').forEach((el) => el.remove());
        document.querySelectorAll('.cell.next-move-played').forEach((el) => el.classList.remove('next-move-played'));

        document.querySelectorAll('.cell').forEach((cell) => {
            const index = parseInt(cell.dataset.index, 10);
            const disc = board.getDisc(index);
            const existingPiece = cell.querySelector('.piece');

            if (existingPiece) {
                if (disc === 'empty') {
                    cell.removeChild(existingPiece);
                } else {
                    existingPiece.className = `piece ${disc}`;
                }
            } else if (disc !== 'empty') {
                const piece = document.createElement('div');
                piece.className = `piece ${disc}`;
                cell.appendChild(piece);
            }
            cell.classList.remove('valid-move');
        });

        const boardEl = document.getElementById('board');
        boardEl.classList.toggle('black-turn', board.blackTurn);
        boardEl.classList.toggle('white-turn', !board.blackTurn);
        // Exploring off the PGN line is shown as a lighter board shade (like chess.com's analysis
        // board) rather than text in the status line, which otherwise always reads "<color> to
        // move" / "<color> has N moves" for whichever board is displayed, on-line or not.
        boardEl.classList.toggle('exploring', this.pgnIsDiverged());

        this.updateScore(board);
        this.updateGameStatus(board);

        if (!this.pgnIsDiverged()) {
            const playedIndex = this.pgnFindPlayedMove(this.pgnCurrentPly);
            if (playedIndex >= 0) {
                const displayIndex = this.flipped ? 63 - playedIndex : playedIndex;
                const cell = document.querySelector(`.cell[data-index="${displayIndex}"]`);
                if (cell) cell.classList.add('next-move-played');
            }
        }

        if (this.evalMode) {
            this.requestMissingEvaluations(board);
            this.renderEvaluations(board);
            // While exploring, also prefetch grandchildren so each further click into the
            // explored subtree has its move evaluations ready immediately, same as normal play.
            if (this.pgnIsDiverged()) this.requestGrandchildrenEvaluations(board);
        }
    }

    pgnFindPlayedMove(ply) {
        if (ply + 1 >= this.pgnBoards.length) return -1;
        const board = this.pgnBoards[ply];
        const nextBoard = this.pgnBoards[ply + 1];
        if (!board || !nextBoard) return -1;
        const nextNorm = nextBoard.normalize().toString();
        for (let i = 0; i < 64; i++) {
            const child = board.doMove(i);
            if (!child) continue;
            if (child.normalize().toString() === nextNorm) return i;
        }
        return -1;
    }

    // ── Eval graph ────────────────────────────────────────────────────────────

    pgnGetGraphData() {
        const result = [];
        for (let i = 0; i < this.pgnBoards.length; i++) {
            const board = this.pgnBoards[i];
            if (!board.hasValidMoves()) { result.push(null); continue; }

            const children = this.pgnChildrenByPly[i];
            if (!children.length) { result.push(null); continue; }

            const childScores = children.map((s) => {
                const e = this.evaluations.get(s);
                return e ? e.score : null;
            }).filter((s) => s !== null);

            if (!childScores.length) { result.push(null); continue; }

            // min(child scores) = mover's best; convert to black's POV.
            const blackPov = board.blackTurn ? -Math.min(...childScores) : Math.min(...childScores);
            result.push({ score: blackPov, blackTurn: board.blackTurn });
        }
        return result;
    }

    pgnRenderGraph() {
        const canvas = document.getElementById('score-graph');
        if (!canvas) return;

        const data = this.pgnGetGraphData();
        const knownScores = data.filter(Boolean).map((d) => d.score);
        if (!knownScores.length) return;

        const dpr = window.devicePixelRatio || 1;
        const rect = canvas.getBoundingClientRect();
        canvas.width = rect.width * dpr;
        canvas.height = rect.height * dpr;
        const ctx = canvas.getContext('2d');
        ctx.scale(dpr, dpr);

        const W = rect.width, H = rect.height;
        const PAD = { top: 10, right: 16, bottom: 24, left: 30 };
        const plotW = W - PAD.left - PAD.right;
        const plotH = H - PAD.top - PAD.bottom;

        const rawMin = Math.min(...knownScores, 0);
        const rawMax = Math.max(...knownScores, 0);
        const yMin = rawMin - 2;
        const yMax = rawMax + 2;
        const yRange = yMax - yMin;

        const interval = yRange <= 20 ? 4 : yRange <= 40 ? 8 : yRange <= 80 ? 16 : 32;

        const xScale = (i) => PAD.left + (data.length <= 1 ? plotW / 2 : (i / (data.length - 1)) * plotW);
        const yScale = (s) => PAD.top + plotH - ((s - yMin) / yRange) * plotH;

        ctx.fillStyle = getComputedStyle(canvas).getPropertyValue('--panel-bg').trim() || '#1a1a2e';
        ctx.fillRect(0, 0, W, H);

        ctx.strokeStyle = 'rgba(255,255,255,0.08)';
        ctx.lineWidth = 1;
        ctx.fillStyle = '#888';
        ctx.font = `${10 * Math.min(1, W / 400)}px sans-serif`;
        ctx.textAlign = 'right';

        const firstGrid = Math.ceil(yMin / interval) * interval;
        for (let v = firstGrid; v <= yMax; v += interval) {
            const y = yScale(v);
            ctx.beginPath();
            ctx.moveTo(PAD.left, y);
            ctx.lineTo(W - PAD.right, y);
            ctx.stroke();
            ctx.fillText(v > 0 ? `+${v}` : `${v}`, PAD.left - 3, y + 3);
        }

        if (yMin <= 0 && yMax >= 0) {
            ctx.strokeStyle = 'rgba(255,255,255,0.25)';
            ctx.lineWidth = 1.5;
            ctx.beginPath();
            ctx.moveTo(PAD.left, yScale(0));
            ctx.lineTo(W - PAD.right, yScale(0));
            ctx.stroke();
        }

        ctx.fillStyle = '#888';
        ctx.textAlign = 'center';
        const xStep = Math.max(1, Math.round(data.length / 8));
        for (let i = 0; i < data.length; i += xStep) {
            ctx.fillText(i, xScale(i), H - PAD.bottom + 12);
        }

        // Score line: connect known-score positions with a continuous line, skipping over
        // null entries (pass/game-over plies) without breaking the path, and drawing no dot
        // for them (see the dot loop below) — the x-position they'd occupy is left as-is so
        // ply spacing along the axis doesn't compress.
        ctx.strokeStyle = 'rgba(100,180,255,0.6)';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        let inPath = false;
        for (let i = 0; i < data.length; i++) {
            if (!data[i]) continue;
            const x = xScale(i), y = yScale(data[i].score);
            if (!inPath) { ctx.moveTo(x, y); inPath = true; }
            else { ctx.lineTo(x, y); }
        }
        ctx.stroke();

        for (let i = 0; i < data.length; i++) {
            if (!data[i]) continue;
            const x = xScale(i), y = yScale(data[i].score);
            const isCurrent = i === this.pgnCurrentPly;
            const r = isCurrent ? 5.5 : 3.5;

            ctx.beginPath();
            ctx.arc(x, y, r, 0, Math.PI * 2);
            ctx.fillStyle = data[i].blackTurn ? '#222' : '#eee';
            ctx.fill();
            ctx.strokeStyle = data[i].blackTurn ? '#aaa' : '#444';
            ctx.lineWidth = isCurrent ? 2 : 1;
            ctx.stroke();
        }

        this._graphData = data;
        this._graphLayout = { xScale };

        if (!this._graphClickBound) {
            this._graphClickBound = true;
            canvas.addEventListener('click', (e) => this.onGraphClick(e));
        }
    }

    onGraphClick(e) {
        if (!this._graphData || !this._graphLayout) return;
        const canvas = document.getElementById('score-graph');
        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;

        const { xScale } = this._graphLayout;
        const data = this._graphData;

        let closest = -1, closestDist = Infinity;
        for (let i = 0; i < data.length; i++) {
            if (!data[i]) continue;
            const dist = Math.abs(xScale(i) - mx);
            if (dist < closestDist) { closestDist = dist; closest = i; }
        }
        if (closest >= 0 && closestDist < 20) {
            // The graph always represents the PGN line, so clicking it leaves any exploration
            // and jumps to the clicked ply on the line.
            this.pgnAlternativeMoves = [];
            this.pgnGoTo(closest);
        }
    }
}

// In the browser (loaded via <script>) there is no CommonJS module system, so
// bootstrap the page. Under Node (e.g. the static/test harness) `module` exists;
// skip the DOM bootstrap and export the pure classes for testing instead.
if (typeof module === 'undefined') {
    new OthelloGame();
} else {
    module.exports = { OthelloBoard, OthelloGame, popcount, rotateBits, LOCAL_EVAL_LEVELS, localEvalLevelsFor };
}
