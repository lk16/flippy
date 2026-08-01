const BITBOARD_MASK = 0xFFFFFFFFFFFFFFFFn;

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
                this.send(this.pendingBoards);
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
        if (boards.length === 0) {
            return;
        }

        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            this.pendingBoards = this.pendingBoards ? [...this.pendingBoards, ...boards] : boards;
            return;
        }

        this.send(boards);
    }

    send(boards) {
        this.ws.send(JSON.stringify({
            id: this.messageId++,
            event: 'evaluation_request',
            data: { boards },
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
        this.evaluations = new Map(); // board string -> {level, depth, confidence, score, source}
        this.wsClient = new WebSocketClient((evaluations) => this.handleEvaluations(evaluations));
        this.board = new OthelloBoard();
        this.boardHistory = [];
        this.evalMode = true;
        this.initializeBoard();
        this.initializeButtons();
        this.renderBoard(null, false);
    }

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
    }

    toggleEvalMode() {
        this.evalMode = !this.evalMode;
        const button = document.getElementById('eval-mode-button');
        button.classList.toggle('active', this.evalMode);
        button.textContent = this.evalMode ? 'Hide evals' : 'Show evals';

        if (this.evalMode) {
            this.requestMissingEvaluations();
            this.renderEvaluations();
        } else {
            document.querySelectorAll('.cell .score-display').forEach((el) => el.remove());
            document.querySelectorAll('.best-move-circle').forEach((el) => el.remove());
            const validMoves = this.board.getValidMoves();
            document.querySelectorAll('.cell').forEach((cell) => {
                const index = parseInt(cell.dataset.index, 10);
                cell.classList.toggle('valid-move', ((1n << BigInt(index)) & validMoves) !== 0n);
            });
        }
    }

    newGame() {
        this.board = new OthelloBoard();
        this.boardHistory = [];
        this.renderBoard(null, false);
    }

    onCellClick(index) {
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

        this.updateValidMoves();
        this.updateScore();
        this.updateGameStatus();
        this.renderEvaluations();
    }

    updateValidMoves() {
        const boardElement = document.getElementById('board');
        boardElement.classList.toggle('black-turn', this.board.blackTurn);
        boardElement.classList.toggle('white-turn', !this.board.blackTurn);

        const validMoves = this.board.getValidMoves();
        document.querySelectorAll('.cell').forEach((cell) => {
            const index = parseInt(cell.dataset.index, 10);
            cell.classList.toggle('valid-move', ((1n << BigInt(index)) & validMoves) !== 0n);
        });

        this.requestMissingEvaluations();
    }

    requestMissingEvaluations() {
        if (!this.evalMode) return;
        const boards = [...new Set(
            this.board.getChildren()
                .map((child) => child.normalize().toString())
                .filter((board) => !this.evaluations.has(board)),
        )];
        this.wsClient.requestEvaluations(boards);
    }

    handleEvaluations(evaluations) {
        for (const evaluation of evaluations) {
            this.evaluations.set(evaluation.board, evaluation);
        }
        this.renderEvaluations();
    }

    // renderEvaluations shows each legal move's score, the negation of its child's stored score.
    // In game mode this is a no-op; the caller already cleared score displays before calling.
    renderEvaluations() {
        if (!this.evalMode) return;
        let bestScore = -Infinity;
        const moveEvaluations = new Map();
        let haveAllEvaluations = true;

        for (let index = 0; index < 64; index++) {
            const child = this.board.doMove(index);
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

    updateScore() {
        document.getElementById('black-score').textContent = this.board.countDiscs('black');
        document.getElementById('white-score').textContent = this.board.countDiscs('white');
    }

    updateGameStatus() {
        const blackCount = this.board.countDiscs('black');
        const whiteCount = this.board.countDiscs('white');
        const statusElement = document.getElementById('game-status');

        if (this.board.isGameOver()) {
            if (blackCount > whiteCount) {
                statusElement.textContent = 'Game Over - Black Wins!';
            } else if (whiteCount > blackCount) {
                statusElement.textContent = 'Game Over - White Wins!';
            } else {
                statusElement.textContent = 'Game Over - Draw!';
            }
            return;
        }

        const validMoves = this.board.countMoves();
        const currentPlayer = this.board.blackTurn ? 'Black' : 'White';
        statusElement.textContent = `${currentPlayer} has ${validMoves} move${validMoves === 1 ? '' : 's'}`;
    }
}

new OthelloGame();
