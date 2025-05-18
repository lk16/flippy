export type DiscColor = 'black' | 'white' | 'empty'

const flipDirections = [
  [-1, -1],
  [-1, 0],
  [-1, 1],
  [0, -1],
  [0, 1],
  [1, -1],
  [1, 0],
  [1, 1],
]

function countBits(bits: bigint): number {
  let count = 0
  let n = bits
  while (n > 0n) {
    count += Number(n & 1n)
    n >>= 1n
  }
  return count
}

export class Board {
  playerBits: bigint
  opponentBits: bigint
  blackTurn: boolean

  constructor() {
    this.playerBits = BigInt(0)
    this.opponentBits = BigInt(0)
    this.blackTurn = true
  }

  static empty(): Board {
    return new Board()
  }

  static start(): Board {
    const board = new Board()

    // Set initial pieces
    board.setDisc(3 * 8 + 3, 'white')
    board.setDisc(3 * 8 + 4, 'black')
    board.setDisc(4 * 8 + 3, 'black')
    board.setDisc(4 * 8 + 4, 'white')
    return board
  }

  clone(): Board {
    const board = new Board()
    board.playerBits = this.playerBits
    board.opponentBits = this.opponentBits
    board.blackTurn = this.blackTurn
    return board
  }

  getDisc(index: number): DiscColor {
    const bit = BigInt(1) << BigInt(index)
    if (this.playerBits & bit) {
      return this.blackTurn ? 'black' : 'white'
    }
    if (this.opponentBits & bit) {
      return this.blackTurn ? 'white' : 'black'
    }
    return 'empty'
  }

  setDisc(index: number, color: DiscColor) {
    const bit = BigInt(1) << BigInt(index)
    if (color === 'black') {
      this.playerBits |= bit
    } else {
      this.opponentBits |= bit
    }
  }

  toString(): string {
    const playerStr = this.playerBits.toString(16).padStart(16, '0')
    const opponentStr = this.opponentBits.toString(16).padStart(16, '0')
    return `${playerStr}${opponentStr}`
  }

  blackDiscs(): bigint {
    return this.blackTurn ? this.playerBits : this.opponentBits
  }

  whiteDiscs(): bigint {
    return this.blackTurn ? this.opponentBits : this.playerBits
  }

  empties(): bigint {
    return ~(this.playerBits | this.opponentBits)
  }

  isEmpty(index: number): boolean {
    let mask = BigInt(1) << BigInt(index)
    return ((this.playerBits | this.opponentBits) & mask) === 0n
  }

  countDiscs(color: DiscColor): number {
    if (color === 'black') {
      return countBits(this.blackDiscs())
    } else if (color === 'white') {
      return countBits(this.whiteDiscs())
    }
    return countBits(this.empties())
  }

  countMoves(): number {
    let count = 0
    for (let i = 0; i < 64; i++) {
      if (this.isValidMove(i)) {
        count++
      }
    }
    return count
  }

  getWinner(): DiscColor | null {
    let passed = this.clone()
    passed.switchTurn()

    // If player or opponent has moves, game is not over
    if (this.hasMoves() || passed.hasMoves()) {
      return null
    }

    let blackScore = this.countDiscs('black')
    let whiteScore = this.countDiscs('white')

    if (blackScore > whiteScore) {
      return 'black'
    } else if (whiteScore > blackScore) {
      return 'white'
    }

    // We indicate a draw by returning 'empty'
    return 'empty'
  }

  hasMoves(): boolean {
    for (let i = 0; i < 64; i++) {
      if (this.isValidMove(i)) {
        return true
      }
    }
    return false
  }

  isValidMove(index: number): boolean {
    if (!this.isEmpty(index)) {
      return false
    }

    for (const [dx, dy] of flipDirections) {
      if (this.getFlippedPerDirection(index, dx, dy) > 0n) {
        return true
      }
    }

    return false
  }

  getFlippedPerDirection(index: number, dx: number, dy: number): bigint {
    if (!this.isEmpty(index)) {
      return 0n
    }

    const row = index >> 3
    const col = index & 7

    let flips = 0n
    let x = row + dx
    let y = col + dy

    const own_color = this.blackTurn ? 'black' : 'white'

    while (x >= 0 && x < 8 && y >= 0 && y < 8) {
      const currentIndex = x * 8 + y
      const currentBit = 1n << BigInt(currentIndex)

      const disc = this.getDisc(currentIndex)

      if (disc === 'empty') {
        return 0n
      }

      if (disc === own_color) {
        return flips
      }

      flips |= currentBit
      x += dx
      y += dy
    }

    return 0n
  }

  switchTurn(): void {
    // Switch turn
    this.blackTurn = !this.blackTurn

    // Swap player and opponent bits
    let temp = this.playerBits
    this.playerBits = this.opponentBits
    this.opponentBits = temp
  }

  doMove(index: number): Board | null {
    if (!this.isEmpty(index)) {
      return null
    }

    let flippedDiscs = 0n
    for (const [dx, dy] of flipDirections) {
      flippedDiscs |= this.getFlippedPerDirection(index, dx, dy)
    }

    if (flippedDiscs === 0n) {
      return null
    }

    const moveBit = BigInt(1) << BigInt(index)

    let child = this.clone()
    child.playerBits |= moveBit | flippedDiscs
    child.opponentBits &= ~flippedDiscs
    child.switchTurn()

    // Pass turn if no moves
    if (!child.hasMoves()) {
      child.switchTurn()

      // If neither player has moves, pass again, this is game over
      if (!child.hasMoves()) {
        child.switchTurn()
      }
    }

    return child
  }

  normalize(): Board {
    // Find the minimum position
    let minBoard = this.clone()
    for (let i = 1; i < 8; i++) {
      const rotatedBoard = Board.rotate(this, i)

      if (rotatedBoard.isLessThan(minBoard)) {
        minBoard = rotatedBoard
      }
    }

    return minBoard
  }

  private static rotate(board: Board, rotation: number): Board {
    let rotated = new Board()
    rotated.playerBits = Board.rotateBits(board.playerBits, rotation)
    rotated.opponentBits = Board.rotateBits(board.opponentBits, rotation)
    return rotated
  }

  private static rotateBits(x: bigint, rotation: number): bigint {
    if (rotation & 1) {
      x = Board.flipHorizontally(x)
    }
    if (rotation & 2) {
      x = Board.flipVertically(x)
    }
    if (rotation & 4) {
      x = Board.flipDiagonally(x)
    }
    return x
  }

  private static flipHorizontally(x: bigint): bigint {
    const k1 = 0x5555555555555555n
    const k2 = 0x3333333333333333n
    const k4 = 0x0f0f0f0f0f0f0f0fn

    x = ((x >> 1n) & k1) | ((x & k1) << 1n)
    x = ((x >> 2n) & k2) | ((x & k2) << 2n)
    x = ((x >> 4n) & k4) | ((x & k4) << 4n)
    return x
  }

  private static flipVertically(x: bigint): bigint {
    const k1 = 0x00ff00ff00ff00ffn
    const k2 = 0x0000ffff0000ffffn

    x = ((x >> 8n) & k1) | ((x & k1) << 8n)
    x = ((x >> 16n) & k2) | ((x & k2) << 16n)
    x = (x >> 32n) | (x << 32n)
    return x
  }

  private static flipDiagonally(x: bigint): bigint {
    const k1 = 0x5500550055005500n
    const k2 = 0x3333000033330000n
    const k4 = 0x0f0f0f0f00000000n

    let t = k4 & (x ^ (x << 28n))
    x ^= t ^ (t >> 28n)
    t = k2 & (x ^ (x << 14n))
    x ^= t ^ (t >> 14n)
    t = k1 & (x ^ (x << 7n))
    x ^= t ^ (t >> 7n)
    return x
  }

  private isLessThan(other: Board): boolean {
    if (this.playerBits < other.playerBits) {
      return true
    }
    if (this.playerBits === other.playerBits && this.opponentBits < other.opponentBits) {
      return true
    }
    return false
  }
}
