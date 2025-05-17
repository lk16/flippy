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
        <div
          v-if="gameStore.board.getDisc(index - 1) !== 'empty'"
          :class="[styles.piece, styles[gameStore.board.getDisc(index - 1)]]"
        />
        <div
          v-else-if="gameStore.board.isValidMove(index - 1)"
          :class="[
            styles['valid-move-indicator'],
            styles[gameStore.board.blackTurn ? 'black-turn' : 'white-turn'],
          ]"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useGameStore } from '~/stores/game'
import styles from './Board.module.css'

const gameStore = useGameStore()
</script>
