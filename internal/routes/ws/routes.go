package ws

import (
	"log/slog"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/services"
	"github.com/lk16/flippy/api/internal/ws"
)

func handleWs(c *websocket.Conn) {
	services := c.Locals("services").(*services.Services) //nolint: errcheck

	h := ws.NewHandler(c, services)
	err := h.Handle()
	if err != nil {
		slog.Error("ws handle error", "error", err)
	}
}

// SetupRoutes sets up the routes for the websocket.
func SetupRoutes(app *fiber.App) {
	app.Get("/ws", websocket.New(handleWs))
}
