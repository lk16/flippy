package game

import (
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/config"
)

func SetupRoutes(app *fiber.App) {
	app.Get("/game", Page)
}

// Page serves the game.html page.
func Page(c *fiber.Ctx) error {
	cfg := c.Locals("config").(*config.ServerConfig) //nolint: errcheck

	return c.SendFile(filepath.Join(cfg.StaticDir, "game.html"))
}
