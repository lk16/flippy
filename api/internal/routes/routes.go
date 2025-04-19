package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/routes/api"
	"github.com/lk16/flippy/api/internal/routes/book"
	"github.com/lk16/flippy/api/internal/routes/clients"
	"github.com/lk16/flippy/api/internal/routes/static"
)

func SetupRoutes(app *fiber.App) {
	// Create API group
	api.SetupRoutes(app)

	// Serve static files
	static.SetupRoutes(app)

	// Serve HTML pages
	book.SetupRoutes(app)
	clients.SetupRoutes(app)
}
