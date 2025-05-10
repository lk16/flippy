package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/routes/api"
	"github.com/lk16/flippy/api/internal/routes/book"
	"github.com/lk16/flippy/api/internal/routes/clients"
	"github.com/lk16/flippy/api/internal/routes/game"
	"github.com/lk16/flippy/api/internal/routes/static"
	"github.com/lk16/flippy/api/internal/routes/version"
	"github.com/lk16/flippy/api/internal/routes/ws"
)

func rootHandler(c *fiber.Ctx) error {
	return c.Redirect("/game")
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

	// Serve websocket routes
	ws.SetupRoutes(app)

	// Serve game routes
	game.SetupRoutes(app)

	// Serve root page
	app.Get("/", rootHandler)
}
