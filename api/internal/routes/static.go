package routes

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
)

const StaticDir = "../src/flippy/book/static"

// StaticHandler serves static files
func StaticHandler() fiber.Handler {
	return filesystem.New(filesystem.Config{
		Root:   http.Dir(StaticDir),
		Browse: false,
	})
}
