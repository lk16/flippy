package evaluate

import (
	"fmt"
	"math/bits"
	"time"

	"github.com/lk16/flippy/api/internal/models"
)

const (
	cornerMask = 0x8100000000000081
)

// Evaluate evaluates a position and returns the score for the player to move.
func Evaluate(position models.NormalizedPosition, depth int) int {
	bot := NewBot()
	pos := position.Position()
	score := bot.alphaBeta(pos, depth, -64, 64)
	bot.printStats()
	return score
}

// Bot is a bot that evaluates othello positions.
type Bot struct {
	startTime time.Time
	nodes     uint64
}

// NewBot creates a new bot.
func NewBot() *Bot {
	return &Bot{
		startTime: time.Now(),
		nodes:     0,
	}
}

func heuristic(pos models.Position) int {
	passed := pos.DoMove(models.PassMove)

	moves := bits.OnesCount64(pos.Moves())
	opponentMoves := bits.OnesCount64(passed.Moves())

	if moves == 0 && opponentMoves == 0 {
		return pos.FinalScore()
	}

	moveDiff := moves - opponentMoves

	playerCorners := bits.OnesCount64(pos.PlayerDiscs() & cornerMask)
	opponentCorners := bits.OnesCount64(pos.OpponentDiscs() & cornerMask)
	cornerDiff := playerCorners - opponentCorners

	return (3 * cornerDiff) + moveDiff
}

func (b *Bot) alphaBeta(pos models.Position, depth int, alpha int, beta int) int {
	b.nodes++

	if depth == 0 {
		return heuristic(pos)
	}

	children := pos.GetChildren()

	if len(children) == 0 {
		passed := pos.DoMove(models.PassMove)

		if !passed.HasMoves() {
			return -pos.FinalScore()
		}

		return -b.alphaBeta(passed, depth, -beta, -alpha)
	}

	for _, child := range children {
		score := -b.alphaBeta(child, depth-1, -beta, -alpha)

		if score >= beta {
			return beta
		}

		if score > alpha {
			alpha = score
		}
	}

	return alpha
}

func (b *Bot) printStats() {
	elapsedSeconds := time.Since(b.startTime).Seconds()

	nodesPerSecond := int64(0)
	if elapsedSeconds > 0.000001 {
		nodesPerSecond = int64(float64(b.nodes) / elapsedSeconds)
	}

	fmt.Printf("Evaluated %d nodes in %.4fs (%d nodes/s)\n", b.nodes, elapsedSeconds, nodesPerSecond)
}
