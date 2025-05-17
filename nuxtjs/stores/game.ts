import { defineStore } from 'pinia'
import { reactive, computed, watch } from 'vue'
import { Board } from '~/types/Board'
import { useWebSocket } from '~/composables/useWebSocket'

export const useGameStore = defineStore('game', () => {
  const state = reactive({
    boardHistory: [Board.start()] as Board[],
    evaluations: new Map<string, number>(),
  })

  const { requestEvaluations } = useWebSocket()

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
    state.evaluations.clear()
  }

  function getBoard() {
    return state.boardHistory[state.boardHistory.length - 1]
  }

  async function updateEvaluations() {
    const board = getBoard()
    if (!board.hasMoves()) return

    // Get all valid moves and their resulting positions
    const positions: string[] = []
    for (let i = 0; i < 64; i++) {
      if (board.isValidMove(i)) {
        const child = board.doMove(i)
        if (child) {
          positions.push(child.normalize().toString())
        }
      }
    }

    // Request evaluations for all positions
    const newEvaluations = await requestEvaluations(positions)
    newEvaluations.forEach((score, position) => {
      state.evaluations.set(position, score)
    })
  }

  function doMove(index: number) {
    const child = getBoard().doMove(index)
    if (child) {
      state.boardHistory.push(child)
      updateEvaluations()
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
    updateEvaluations()
  }

  // Watch for board changes to update evaluations
  watch(
    () => state.boardHistory.length,
    () => {
      updateEvaluations()
    }
  )

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
