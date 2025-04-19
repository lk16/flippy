package clients

import (
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/middleware"
)

func SetupRoutes(app *fiber.App) {
	clientsGroup := app.Group("/clients", middleware.BasicAuth())
	clientsGroup.Get("/", clientsPage)
}

// clientsPage serves the clients.html page
func clientsPage(c *fiber.Ctx) error {
	return c.SendFile(filepath.Join(config.StaticDir, "clients.html"))
}
