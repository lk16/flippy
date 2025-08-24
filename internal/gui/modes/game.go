package modes

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/lk16/flippy/api/internal/models"
)

type Game struct {
	game *models.Game
}

var _ Mode = &Game{}

// TODO add evaluations

func NewGame() *Game {
	return &Game{
		game: models.NewGame(),
	}
}

func (m *Game) GetBoard() models.Board {
	return m.game.LastBoard()
}

func (m *Game) OnMove(index int) {
	_ = m.game.PushMove(index)
}

func (m *Game) OnClick(button rl.MouseButton, _, _ int) {
	if button == rl.MouseRightButton {
		m.game.PopMove()
		return
	}
}

func (m *Game) OnKeyPress(key int) {
	if key == rl.KeyN {
		m.game = models.NewGame()
	}
}

func (m *Game) GetUIOptions() *UIOPtions {
	return nil
}

func (m *Game) OnFrame() {
}
