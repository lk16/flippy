package modes

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/lk16/flippy/api/internal/models"
)

type Evaluate struct {
	game *models.Game
}

func NewEvaluate() *Evaluate {
	return &Evaluate{
		game: models.NewGame(),
	}
}

var _ Mode = &Evaluate{}

func (m *Evaluate) GetBoard() models.Board {
	return m.game.LastBoard()
}

func (m *Evaluate) OnMove(index int) {
	_ = m.game.PushMove(index)
}

func (m *Evaluate) OnClick(button rl.MouseButton, _, _ int) {
	if button == rl.MouseRightButton {
		m.game.PopMove()
		return
	}
}

func (m *Evaluate) OnKeyPress(key int) {
	if key == rl.KeyN {
		m.game = models.NewGame()
	}
}

func (m *Evaluate) GetUIOptions() *UIOPtions {
	return nil
}
