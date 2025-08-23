package modes

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/lk16/flippy/api/internal/models"
)

type GameMode struct {
	// TODO implement with models.Game
	boards []models.Board
	index  int
}

var _ Mode = &GameMode{}

func NewGameMode() *GameMode {
	return &GameMode{
		boards: []models.Board{models.NewBoardStart()},
		index:  0,
	}
}

func (m *GameMode) GetBoard() models.Board {
	return m.boards[m.index]
}

func (m *GameMode) OnMove(index int) {
	board := m.GetBoard()

	if !board.IsValidMove(index) {
		return
	}

	child := board.DoMove(index)

	if !child.HasMoves() {
		passed := child.DoMove(models.PassMove)

		if passed.HasMoves() {
			child = passed
		}
	}

	m.boards = append(m.boards, child)
	m.index = len(m.boards) - 1
}

func (m *GameMode) OnClick(button rl.MouseButton, _, _ int) {
	if button == rl.MouseRightButton {
		m.undoMove()
		return
	}
}

func (m *GameMode) OnKeyPress(key int) {
	if key == rl.KeyN {
		m.resetBoard()
	}
}

func (m *GameMode) GetUIOptions() *UIOPtions {
	return nil
}

func (m *GameMode) resetBoard() {
	m.boards = []models.Board{models.NewBoardStart()}
	m.index = 0
}

func (m *GameMode) undoMove() {
	if m.index == 0 {
		return
	}

	m.index--
	m.boards = m.boards[:m.index+1]
}
