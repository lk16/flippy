package book

import (
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/middleware"
)

// SetupRoutes sets up the routes for the book.
func SetupRoutes(app *fiber.App) {
	bookGroup := app.Group("/book", middleware.BasicAuth())
	bookGroup.Get("/", Page)
}

// Page serves the book.html page.
func Page(c *fiber.Ctx) error {
	return c.SendFile(filepath.Join(config.StaticDir, "book.html"))
}
