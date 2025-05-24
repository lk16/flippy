const BITBOARD_MASK = 0xFFFFFFFFFFFFFFFFn;

class WebSocketClient {
    constructor(game) {
        this.ws = null;
        this.messageId = 1;
        this.game = game;
        this.connect();
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

        this.ws.onopen = () => {
            // Request initial evaluations once connected
            let positions = this.game.board.getChildren()
                .map(child => child.normalize().toString())
                .filter(pos => !this.game.evaluations_map.has(pos));

            this.requestEvaluations(positions);
        };

        this.ws.onmessage = (event) => {
            try {
                const response = JSON.parse(event.data);
                if (response.data && response.data.evaluations) {
                    this.game.handleEdaxWsEvaluations(response.data.evaluations);
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

    requestEvaluations(positions) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            return;
        }

        if (positions.length === 0) {
            return;
        }

        const message = {
            id: this.messageId++,
            event: 'evaluation_request',
            data: {
                positions: positions
            }
        };

        this.ws.send(JSON.stringify(message));
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
    const mask = 0xFFFFFFFFFFFFFFFFn;

    x = ((x >> 8n) & k1) | ((x & k1) << 8n);
    x = ((x >> 16n) & k2) | ((x & k2) << 16n);
    x = (x >> 32n) | (x << 32n);
    return x & mask;
}

function flipDiagonally(x) {
    const k1 = 0x5500550055005500n;
    const k2 = 0x3333000033330000n;
    const k4 = 0x0F0F0F0F00000000n;
    const mask = 0xFFFFFFFFFFFFFFFFn;

    let t = k4 & (x ^ (x << 28n));
    x ^= t ^ (t >> 28n);
    t = k2 & (x ^ (x << 14n));
    x ^= t ^ (t >> 14n);
    t = k1 & (x ^ (x << 7n));
    x ^= t ^ (t >> 7n);
    return x & mask;
}

function rotateBits(x, rotation) {
    if ((rotation & 1) != 0) {
        x = flipHorizontally(x)
    }
    if ((rotation & 2) != 0) {
        x = flipVertically(x)
    }
    if ((rotation & 4) != 0) {
        x = flipDiagonally(x)
    }
    return x
}

class OthelloBoard {
    constructor() {
        this.playerBits = BigInt(0);
        this.opponentBits = BigInt(0);

        this.blackTurn = true;

        // Set initial pieces
        this.setDisc(3 * 8 + 3, 'white');
        this.setDisc(3 * 8 + 4, 'black');
        this.setDisc(4 * 8 + 3, 'black');
        this.setDisc(4 * 8 + 4, 'white');
    }

    clone() {
        const copy = new OthelloBoard();
        Object.assign(copy, this);
        return copy;
    }


    validate() {
        if (typeof this.playerBits !== 'bigint') {
            throw new Error('Player bits is not a bigint');
        }

        if (typeof this.opponentBits !== 'bigint') {
            throw new Error('Opponent bits is not a bigint');
        }

        if ((this.playerBits & BITBOARD_MASK) !== this.playerBits) {
            throw new Error('Player bits out of range');
        }

        if ((this.opponentBits & BITBOARD_MASK) !== this.opponentBits) {
            throw new Error('Opponent bits out of range');
        }

        if (this.playerBits & this.opponentBits) {
            throw new Error('Player and opponent bits overlap');
        }
    }

    getDisc(index) {
        const bit = BigInt(1) << BigInt(index);

        if (this.playerBits & bit) {
            if (this.blackTurn) {
                return 'black';
            } else {
                return 'white';
            }
        }
        if (this.opponentBits & bit) {
            if (this.blackTurn) {
                return 'white';
            } else {
                return 'black';
            }
        }
        return 'empty';
    }

    setDisc(index, player) {
        const bit = BigInt(1) << BigInt(index);
        if (player === 'black') {
            this.playerBits |= bit;
        } else {
            this.opponentBits |= bit;
        }

        this.validate();
    }

    countDiscs(color) {
        if ((color === 'black') === this.blackTurn) {
            return this.playerDiscCount();
        }
        return this.opponentDiscCount();
    }

    playerDiscCount() {
        // Emulate popcount
        let count = 0n;
        let n = this.playerBits;
        while (n > 0n) {
            count += n & 1n;
            n >>= 1n;
        }
        return Number(count);
    }

    opponentDiscCount() {
        // Emulate popcount
        let count = 0n;
        let n = this.opponentBits;
        while (n > 0n) {
            count += n & 1n;
            n >>= 1n;
        }
        return Number(count);
    }

    emptyCount() {
        return 64 - this.opponentDiscCount() - this.playerDiscCount();
    }

    passMove() {
        this.blackTurn = !this.blackTurn;

        // Swap player and opponent bits
        const temp = this.playerBits;
        this.playerBits = this.opponentBits;
        this.opponentBits = temp;

        this.validate();
    }

    hasValidMoves() {
        return [...Array(64).keys()].some(i => this.isValidMove(i));
    }

    isValidMove(index) {
        if (this.getDisc(index) !== 'empty') {
            return false;
        }

        const directions = [
            [-1, -1], [-1, 0], [-1, 1],
            [0, -1], [0, 1],
            [1, -1], [1, 0], [1, 1]
        ];

        return directions.some(([dx, dy]) => this.getFlippedPerDirection(index, dx, dy) > 0n);
    }

    getValidMoves() {
        let moves = 0n;

        for (let i = 0; i < 64; i++) {
            if (this.isValidMove(i)) {
                moves |= (1n << BigInt(i));
            }
        }

        return moves;
    }

    countMoves() {
        const moves = this.getValidMoves();

        let count = 0n;

        // Emulate popcount
        let n = moves;
        while (n > 0n) {
            count += n & 1n;
            n >>= 1n;
        }

        return count;
    }

    isGameOver() {
        if (this.hasValidMoves()) {
            return false;
        }

        const clone = this.clone();
        clone.passMove();

        return !clone.hasValidMoves();
    }

    getFlippedPerDirection(index, dx, dy) {
        if (this.getDisc(index) !== 'empty') {
            return 0n;
        }

        const row = index >> 3;
        const col = index & 7;

        let flips = 0n;
        let x = row + dx;
        let y = col + dy;

        const own_color = this.blackTurn ? 'black' : 'white';

        while (x >= 0 && x < 8 && y >= 0 && y < 8) {
            const currentIndex = x * 8 + y;
            const currentBit = 1n << BigInt(currentIndex);

            const disc = this.getDisc(currentIndex);

            if (disc === 'empty') {
                return 0n;
            }

            if (disc === own_color) {
                return flips;
            }

            flips |= currentBit;
            x += dx;
            y += dy;
        }

        // We walked off the board
        return 0n;
    }

    getFlipped(index) {
        const directions = [
            [-1, -1], [-1, 0], [-1, 1],
            [0, -1], [0, 1],
            [1, -1], [1, 0], [1, 1]
        ];

        let flipped = 0n;
        for (const [dx, dy] of directions) {
            flipped |= this.getFlippedPerDirection(index, dx, dy);
        }

        return flipped;
    }

    doMove(index) {
        if (index < 0 || index >= 64) {
            return null;
        }

        let flipped = this.getFlipped(index);

        if (flipped === 0n) {
            return null;
        }

        const child = new OthelloBoard();
        child.opponentBits = this.playerBits | flipped | (1n << BigInt(index));
        child.playerBits = this.opponentBits & ~child.opponentBits;
        child.blackTurn = !this.blackTurn;

        child.validate();

        return child;
    }

    getChildren() {
        return [...Array(64).keys()].map(i => this.doMove(i)).filter(Boolean);
    }

    rotate(r) {
        const rotated = new OthelloBoard();
        rotated.playerBits = rotateBits(this.playerBits, r);
        rotated.opponentBits = rotateBits(this.opponentBits, r);
        rotated.blackTurn = this.blackTurn;
        rotated.validate();
        return rotated;
    }

    isLessThan(other) {
        if (this.playerBits < other.playerBits) {
            return true;
        }

        if (this.playerBits == other.playerBits && this.opponentBits < other.opponentBits) {
            return true;
        }

        return false;
    }

    normalize() {
        let min = this.clone();

        // `min` starts as a copy of `this`, which is the rotation of 0 (no rotation)
        for (let r = 1; r < 8; r++) {
            const rotated = this.rotate(r);
            if (rotated.isLessThan(min)) {
                min = rotated;
            }
        }

        return min;
    }

    toString() {
        const playerStr = this.playerBits.toString(16).padStart(16, '0');
        const opponentStr = this.opponentBits.toString(16).padStart(16, '0');
        return (playerStr + opponentStr).toUpperCase();
    }

    finalScore() {
        const player = Number(this.playerDiscCount());
        const opponent = Number(this.opponentDiscCount());

        // Player wins
        if (player > opponent) {
            return 64 - (2 * opponent);
        }

        // Opponent wins
        if (player < opponent) {
            return 64 - (2 * player);
        }

        // Draw
        return 0;
    }
}

class OthelloGame {
    constructor() {
        this.wsClient = new WebSocketClient(this);
        this.board = new OthelloBoard();
        this.boardHistory = []; // Store previous board states
        this.evaluations_map = new Map();
        this.initializeBoard();
        this.initializeButtons();
        this.renderBoard(null, false); // No animation on initial load
    }

    initializeBoard() {
        const boardElement = document.getElementById('board');
        boardElement.innerHTML = '';

        // Add right click handler to the entire board
        boardElement.addEventListener('contextmenu', (e) => {
            e.preventDefault(); // Prevent the context menu from appearing
            this.undoMove();
        });

        for (let index = 0; index < 64; index++) {
            const cell = document.createElement('div');
            cell.className = 'cell';
            cell.dataset.index = index;

            // Add left click handler
            cell.addEventListener('click', () => this.onDoMoveClick(index));

            boardElement.appendChild(cell);
        }
    }

    initializeButtons() {
        document.getElementById('undo-button').addEventListener('click', () => this.undoMove());
        document.getElementById('new-game-button').addEventListener('click', () => this.newGame());
    }

    newGame() {
        this.board = new OthelloBoard();
        this.boardHistory = [];
        this.renderBoard(null, false);
    }

    renderBoard(previousBoard, animate) {
        // Clear all evaluation scores
        document.querySelectorAll('.cell .score-display').forEach(display => display.remove());
        document.querySelectorAll('.best-move-circle').forEach(circle => circle.remove());

        const cells = document.querySelectorAll('.cell');

        cells.forEach(cell => {
            const index = parseInt(cell.dataset.index);
            const disc = this.board.getDisc(index);
            const previousDisc = previousBoard ? previousBoard.getDisc(index) : null;

            // Handle existing piece
            const existingPiece = cell.querySelector('.piece');
            if (existingPiece) {
                if (disc === 'empty') {
                    if (animate) {
                        // Fade out smoothly
                        existingPiece.style.opacity = '0';
                        setTimeout(() => {
                            if (existingPiece.parentNode === cell) {
                                cell.removeChild(existingPiece);
                            }
                        }, 300);
                    } else {
                        cell.removeChild(existingPiece);
                    }
                } else {
                    // Always update the piece color if there's a disc
                    existingPiece.classList.remove('black', 'white');
                    existingPiece.classList.add(disc);
                }
            } else if (disc !== 'empty') {
                // Add new piece
                const piece = document.createElement('div');
                piece.className = `piece ${disc}`;
                if (animate) {
                    piece.style.opacity = '0';
                    cell.appendChild(piece);
                    // Force reflow
                    piece.offsetHeight;
                    // Fade in
                    piece.style.opacity = '1';
                } else {
                    cell.appendChild(piece);
                }
            }
        });

        // Update rest of the UI
        this.updateValidMoves();
        this.updateScore();
        this.updateGameStatus();
        this.renderEvaluations();
    }

    onDoMoveClick(index) {
        const child = this.board.doMove(index);

        if (!child) {
            return;
        }

        // Store current board state in history before making the move
        this.boardHistory.push(this.board.clone());
        const previousBoard = this.board.clone();

        // Update current board
        this.board = child;

        if (!this.board.hasValidMoves()) {
            // Store board state before passing
            this.boardHistory.push(this.board.clone());

            // No moves for current player, switch to opponent
            this.board.passMove();

            if (!this.board.hasValidMoves()) {
                // Store board state before second pass
                this.boardHistory.push(this.board.clone());

                // No moves for opponent either, game over
                // Switch back to current player
                this.board.passMove();
            }
        }

        // Render the board with the previous state for animations
        this.renderBoard(previousBoard, true);
    }

    undoMove() {
        if (this.boardHistory.length === 0) {
            return; // No moves to undo
        }

        // Clear all evaluation scores
        document.querySelectorAll('.cell .score-display').forEach(display => display.remove());
        document.querySelectorAll('.best-move-circle').forEach(circle => circle.remove());

        // Store current state for animation
        const previousState = this.board.clone();

        // Pop states until we find one with valid moves
        while (this.boardHistory.length > 0) {
            this.board = this.boardHistory.pop();

            // Undo until we find a board with moves
            if (this.board.hasValidMoves()) {
                break;
            }
        }

        // Update the display with previous state for animations
        this.renderBoard(previousState, true);
    }

    updateValidMoves() {
        const cells = document.querySelectorAll('.cell');
        const board = document.getElementById('board');

        // Update the board's class based on current turn
        board.classList.remove('black-turn', 'white-turn');
        board.classList.add(this.board.blackTurn ? 'black-turn' : 'white-turn');

        const validMoves = this.board.getValidMoves();

        cells.forEach(cell => {
            const index = parseInt(cell.dataset.index);
            const isValidMove = ((1n << BigInt(index)) & validMoves) !== 0n;
            cell.classList.toggle('valid-move', isValidMove);
        });

        if (validMoves !== 0n) {
            let positions = this.board.getChildren()
                .map(child => child.normalize().toString())
                .filter(pos => !this.evaluations_map.has(pos));
            positions = [...new Set(positions)];

            this.wsClient.requestEvaluations(positions);
        }
    }

    handleEdaxWsEvaluations(evaluations) {
        for (const evaluation of evaluations) {
            const value = {
                source: 'edax_ws',
                data: evaluation
            }

            this.evaluations_map.set(evaluation.position, value);
        }

        // Get all normalized children positions that are not in evaluations_map
        const normalizedChildren = this.board.getChildren()
            .map(child => child.normalize())
            .filter(n_child => !this.evaluations_map.has(n_child.toString()));

        // Remove duplicates
        const uniquePositions = [...new Set(normalizedChildren)];

        // Find out depth for WebAssembly evaluation
        const empties = this.board.emptyCount();
        const depth = empties < 12 ? 12 : 6;

        // Evaluate each position
        for (const board of uniquePositions) {
            // This calls the WebAssembly function
            const score = evaluate_position(board.toString(), depth);

            this.evaluations_map.set(board.toString(), {
                source: 'wasm',
                data: {
                    score: score
                }
            });
        }

        this.renderEvaluations();
    }

    renderEvaluations() {
        let highestScore = -Infinity;

        // Map of move index to score
        const moveScores = new Map();

        // Only highlight best moves if we have evaluations for all children
        let showBestMoves = true;

        for (let moveIndex = 0; moveIndex < 64; moveIndex++) {
            const child = this.board.doMove(moveIndex);

            // Field is not a valid move
            if (!child) {
                continue;
            }

            const entry = this.evaluations_map.get(child.normalize().toString());

            // Evaluation is not available for this move
            if (!entry) {
                showBestMoves = false;
                continue;
            }

            if (entry.source !== 'edax_ws' && entry.source !== 'wasm') {
                console.error("Unhandled evaluation source", entry);
                continue;
            }

            let score = -entry.data.score; // Invert score for current player's perspective
            highestScore = Math.max(highestScore, score);

            moveScores.set(moveIndex, entry);
        }

        // Update UI for all cells
        const cells = document.querySelectorAll('.cell');
        cells.forEach(cell => {
            const index = parseInt(cell.dataset.index);
            const entry = moveScores.get(index);

            // Remove existing circle if present
            const existingCircle = cell.querySelector('.best-move-circle');
            if (existingCircle) {
                cell.removeChild(existingCircle);
            }

            if (entry === undefined) {
                return;
            }

            // Invert score for current player's perspective
            let score = -entry.data.score;

            let source = entry.source;

            cell.classList.remove('valid-move');

            // Update score display
            let scoreDisplay = cell.querySelector('.score-display');
            if (!scoreDisplay) {
                scoreDisplay = document.createElement('div');
                scoreDisplay.className = 'score-display';
                cell.appendChild(scoreDisplay);
            }
            scoreDisplay.textContent = score > 0 ? `+${score}` : score;

            if (source === 'edax_ws') {
                scoreDisplay.style.color = this.board.blackTurn ? '#000000' : '#ecf0f1';
            } else if (source === 'wasm') {
                // Show more grayish score, because it's not as reliable
                scoreDisplay.style.color = this.board.blackTurn ? '#333333' : '#999999';
            } else {
                console.error("Unhandled evaluation source", data);
                return;
            }

            // Add circle for best moves only if we have all evaluations
            if (showBestMoves && score === highestScore) {
                const circle = document.createElement('div');
                circle.className = 'best-move-circle';
                if (source === 'edax_ws') {
                    circle.style.borderColor = this.board.blackTurn ? '#000000' : '#ecf0f1';
                } else if (source === 'wasm') {
                    circle.style.borderColor = this.board.blackTurn ? '#333333' : '#999999';
                }
                cell.appendChild(circle);
            }
        });
    }

    updateScore() {
        const blackCount = this.board.countDiscs('black');
        const whiteCount = this.board.countDiscs('white');

        document.getElementById('black-score').textContent = blackCount;
        document.getElementById('white-score').textContent = whiteCount;
    }

    updateGameStatus() {
        const blackCount = this.board.countDiscs('black');
        const whiteCount = this.board.countDiscs('white');
        const validMoves = this.board.countMoves();

        const statusElement = document.getElementById('game-status');
        if (this.board.isGameOver()) {
            if (blackCount > whiteCount) {
                statusElement.textContent = 'Game Over - Black Wins!';
            } else if (whiteCount > blackCount) {
                statusElement.textContent = 'Game Over - White Wins!';
            } else {
                statusElement.textContent = 'Game Over - Draw!';
            }
        } else {
            const currentPlayer = this.board.blackTurn ? 'Black' : 'White';
            statusElement.textContent = `${currentPlayer} has ${validMoves} move${validMoves === 1n ? '' : 's'}`;
        }
    }
}

// Initialize the game when the page loads
window.startOthelloGame = function () {
    new OthelloGame();
}
