package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/middleware"
	"github.com/lk16/flippy/api/internal/routes/api"
)

func SetupRoutes(app *fiber.App) {
	// Create API group with auth middleware
	api.SetupRoutes(app)

	// Serve static files
	app.Use("/static", StaticHandler())

	// HTML routes with basic auth
	htmlGroup := app.Group("", middleware.BasicAuth())
	htmlGroup.Get("/", ClientsPage)
	htmlGroup.Get("/book", BookPage)
}
