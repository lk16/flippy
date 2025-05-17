<template>
  <div :class="styles['game-container']">
    <div :class="styles['game-info']">
      <div :class="styles['main-score-display']">
        <div :class="styles['disc-circles']">
          <div :class="[styles['disc-circle'], styles.black]">
            <span>{{ gameStore.blackScore }}</span>
          </div>
          <div :class="styles['disc-separator']">-</div>
          <div :class="[styles['disc-circle'], styles.white]">
            <span>{{ gameStore.whiteScore }}</span>
          </div>
        </div>
        <div :class="styles['game-status']">{{ gameStore.gameStatus }}</div>
      </div>
    </div>
    <OthelloBoard />
    <div :class="styles['button-container']">
      <button :class="styles['game-button']" @click="gameStore.newGame">New Game</button>
      <button :class="styles['game-button']" @click="gameStore.undoMove">Undo</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useGameStore } from '~/stores/game'
import styles from './index.module.css'

const gameStore = useGameStore()

// Initialize the game
gameStore.newGame()
</script>
