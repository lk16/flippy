package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/routes/api"
	"github.com/lk16/flippy/api/internal/routes/book"
	"github.com/lk16/flippy/api/internal/routes/clients"
	"github.com/lk16/flippy/api/internal/routes/static"
	"github.com/lk16/flippy/api/internal/routes/version"
)

func rootHandler(c *fiber.Ctx) error {
	return c.Redirect("/book")
}

func SetupRoutes(app *fiber.App) {
	// Serve API routes
	api.SetupRoutes(app)

	// Serve static files
	static.SetupRoutes(app)

	// Serve HTML pages
	book.SetupRoutes(app)
	clients.SetupRoutes(app)

	// Serve version info
	version.SetupRoutes(app)

	// Serve root page
	app.Get("/", rootHandler)
}
