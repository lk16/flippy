import { defineStore } from 'pinia'
import { reactive, computed } from 'vue'
import { Board } from '~/types/Board'

export const useGameStore = defineStore('game', () => {
  const state = reactive({
    boardHistory: [Board.start()] as Board[],
    evaluations: new Map<string, number>(), // TODO
  })

  const blackScore = computed(() => getBoard().countDiscs('black'))
  const whiteScore = computed(() => getBoard().countDiscs('white'))

  const gameStatus = computed(() => {
    const winner = getBoard().getWinner()

    if (winner === null) {
      const currentPlayer = getBoard().blackTurn ? 'Black' : 'White'
      const moves = getBoard().countMoves()
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

  function startNewGame() {
    state.boardHistory = [Board.start()]
  }

  function getBoard() {
    return state.boardHistory[state.boardHistory.length - 1]
  }

  function doMove(index: number) {
    const child = getBoard().doMove(index)
    if (child) {
      state.boardHistory.push(child)
    }
  }

  function undoMove() {
    if (state.boardHistory.length === 1) {
      return
    }

    let lastMove = state.boardHistory.pop()
    while (state.boardHistory.length > 1 && lastMove !== undefined && !lastMove.hasMoves()) {
      lastMove = state.boardHistory.pop()
    }
  }

  return {
    board: computed(() => getBoard()),
    boardHistory: computed(() => state.boardHistory),
    evaluations: computed(() => state.evaluations),
    blackScore,
    whiteScore,
    gameStatus,
    newGame: startNewGame,
    doMove,
    undoMove,
  }
})
