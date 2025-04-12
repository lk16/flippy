package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/middleware"
)

// SetupRoutes sets up the API routes
func SetupRoutes(app *fiber.App) {
	apiGroup := app.Group("/api", middleware.AuthOrToken())

	// Client routes
	apiGroup.Post("/learn-clients/register", RegisterClient)
	apiGroup.Post("/learn-clients/heartbeat", Heartbeat)
	apiGroup.Get("/learn-clients", GetClients)

	// Job routes
	apiGroup.Get("/job", GetJob)
	apiGroup.Post("/job/result", SubmitJobResult)

	// Evaluation routes
	apiGroup.Post("/evaluations", SubmitEvaluations)

	// Position routes
	apiGroup.Post("/positions/lookup", LookupPositions)

	// Stats routes
	apiGroup.Get("/stats/book", GetBookStats)
}
