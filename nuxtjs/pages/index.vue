<template>
  <div class="game-container">
    <h1>Othello</h1>
    <div class="game-info">
      <div class="main-score-display">
        <div class="disc-circles">
          <div class="disc-circle black">
            <span>{{ gameStore.blackScore }}</span>
          </div>
          <div class="disc-separator">-</div>
          <div class="disc-circle white">
            <span>{{ gameStore.whiteScore }}</span>
          </div>
        </div>
        <div class="game-status">{{ gameStore.gameStatus }}</div>
      </div>
    </div>
    <OthelloBoard />
    <div class="button-container">
      <button class="game-button" @click="gameStore.newGame">New Game</button>
      <button class="game-button" @click="gameStore.undoMove">Undo</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useGameStore } from '~/stores/game'

const gameStore = useGameStore()

// Initialize the game
gameStore.newGame()
</script>

<style scoped>
.game-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20px;
  width: 100%;
  max-width: 100%;
  padding: 0 10px;
  box-sizing: border-box;
}

.game-info {
  display: flex;
  justify-content: center;
  width: 100%;
  padding: 20px 0;
}

.disc-circles {
  display: flex;
  align-items: center;
  gap: 1em;
}

.disc-circle {
  width: 50px;
  height: 50px;
  border-radius: 50%;
  display: flex;
  justify-content: center;
  align-items: center;
  font-size: 24px;
  font-weight: bold;
}

.disc-circle.black {
  background-color: #000000;
  color: #ecf0f1;
}

.disc-circle.white {
  background-color: #ecf0f1;
  color: #000000;
}

.disc-separator {
  font-size: 24px;
  font-weight: bold;
  color: #2e7d32;
}

.game-status {
  font-size: 24px;
  font-weight: bold;
  color: #2e7d32;
  text-align: center;
  margin-top: 10px;
}

.button-container {
  display: flex;
  gap: 10px;
  width: 100%;
  max-width: min(95vw, 700px);
  margin-top: 20px;
  justify-content: center;
}

.game-button {
  flex: 1;
  padding: 12px 20px;
  border: none;
  border-radius: 5px;
  background-color: #2e7d32;
  color: #ecf0f1;
  font-size: 18px;
  cursor: pointer;
  transition: background-color 0.3s ease;
}

.game-button:hover {
  background-color: #3a506b;
}
</style>
