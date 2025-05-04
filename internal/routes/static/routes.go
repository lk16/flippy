package static

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/lk16/flippy/api/internal/config"
)

// staticHandler serves static files.
func staticHandler() fiber.Handler {
	return filesystem.New(filesystem.Config{
		Root:   http.Dir(config.StaticDir),
		Browse: false,
	})
}

func SetupRoutes(app *fiber.App) {
	app.Use("/static", staticHandler())
}
