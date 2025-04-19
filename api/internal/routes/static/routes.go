package static

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
)

const StaticDir = "../src/flippy/book/static"

// staticHandler serves static files
func staticHandler() fiber.Handler {
	return filesystem.New(filesystem.Config{
		Root:   http.Dir(StaticDir),
		Browse: false,
	})
}

func SetupRoutes(app *fiber.App) {
	app.Use("/static", staticHandler())
}
