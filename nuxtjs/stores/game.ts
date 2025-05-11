import { defineStore } from 'pinia'
import { reactive, computed } from 'vue'
import { Board, type DiscColor } from '~/types/Board'

export const useGameStore = defineStore('game', () => {
  const state = reactive({
    board: Board.start(),
    boardHistory: [] as Board[], // TODO use
    evaluations: new Map<string, number>(), // TODO
  })

  const blackScore = computed(() => state.board.countDiscs('black'))
  const whiteScore = computed(() => state.board.countDiscs('white'))

  const gameStatus = computed(() => {
    const winner = state.board.getWinner()

    if (winner === null) {
      const currentPlayer = state.board.blackTurn ? 'Black' : 'White'
      const moves = state.board.countMoves()
      return `${currentPlayer} has ${moves} move${moves === 1 ? '' : 's'}`
    }

    if (winner === 'black') {
      return 'Game Over - Black Wins!'
    }

    if (winner === 'white') {
      return 'Game Over - White Wins!'
    }

    return 'Game Over - Draw!'
  })

  function newGame() {
    state.board = Board.start()
    state.boardHistory = []
  }

  function setDisc(index: number, color: DiscColor) {
    const bit = BigInt(1) << BigInt(index)
    if (color === 'black') {
      state.board.playerBits |= bit
    } else {
      state.board.opponentBits |= bit
    }
  }

  function doMove(index: number) {
    // TODO use history

    const child = state.board.doMove(index)
    if (child) {
      state.board = child
    }
  }

  return {
    board: computed(() => state.board),
    boardHistory: computed(() => state.boardHistory),
    evaluations: computed(() => state.evaluations),
    blackScore,
    whiteScore,
    gameStatus,
    newGame,
    setDisc,
    doMove,
  }
})
