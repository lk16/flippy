<template>
  <div :class="styles['board-container']" @contextmenu.prevent="gameStore.undoMove">
    <div
      :class="[
        styles.board,
        { 'black-turn': gameStore.board.blackTurn, 'white-turn': !gameStore.board.blackTurn },
      ]"
    >
      <div
        v-for="index in 64"
        :key="index - 1"
        :class="styles.cell"
        @click="gameStore.doMove(index - 1)"
      >
        <transition name="disc">
          <div
            v-if="gameStore.board.getDisc(index - 1) !== 'empty'"
            :class="[styles.piece, styles[gameStore.board.getDisc(index - 1)]]"
          />
        </transition>
        <div
          v-if="
            gameStore.board.getDisc(index - 1) === 'empty' && gameStore.board.isValidMove(index - 1)
          "
          :class="[
            styles['valid-move-indicator'],
            styles[gameStore.board.blackTurn ? 'black-turn' : 'white-turn'],
          ]"
        >
          <span v-if="getEvaluation(index - 1)" :class="styles.evaluation">
            {{ getEvaluation(index - 1) }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useGameStore } from '~/stores/game'
import styles from './OthelloBoard.module.css'

const gameStore = useGameStore()

// TODO move into store
function getEvaluation(index: number): number | null {
  const child = gameStore.board.doMove(index)
  if (!child) return null
  return gameStore.evaluations.get(child.normalize().toString()) ?? null
}
</script>
