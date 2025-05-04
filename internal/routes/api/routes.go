package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/middleware"
)

// SetupRoutes sets up the API routes.
func SetupRoutes(app *fiber.App) {
	apiGroup := app.Group("/api", middleware.AuthOrToken())

	// Client routes
	apiGroup.Post("/learn-clients/register", RegisterClient)
	apiGroup.Post("/learn-clients/heartbeat", Heartbeat)
	apiGroup.Get("/learn-clients", GetClients)
	apiGroup.Get("/learn-clients/job", GetJob)

	// Position routes
	apiGroup.Post("/positions/evaluations", SubmitEvaluations)
	apiGroup.Post("/positions/lookup", LookupPositions)
	apiGroup.Get("/positions/stats", GetBookStats)
}
