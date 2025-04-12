package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/lk16/flippy/api/internal/handlers"
	"github.com/lk16/flippy/api/internal/middleware"
	"github.com/lk16/flippy/api/internal/services"
)

func main() {
	// Initialize database
	postgres, err := services.InitPostgres()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize Redis
	redis, err := services.InitRedis()
	if err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}

	// Get the path to the static files
	staticDir := "../src/flippy/book/static"

	// Create handlers
	htmlHandler := handlers.NewHTMLHandler(staticDir)

	// Create auth config
	authConfig := middleware.NewBasicAuthConfig()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		Prefork:      true,
		Concurrency:  256 * 1024, // Maximum number of concurrent connections
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  5 * time.Second,
		BodyLimit:    1024 * 1024, // 1MB
	})

	// Setup connections to external services in Fiber app
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("services", &services.Services{
			Postgres: postgres,
			Redis:    redis,
		})
		return c.Next()
	})

	// Add logging middleware
	app.Use(middleware.Logging())

	// Add CORS middleware
	app.Use(cors.New())

	// Serve static files
	app.Use("/static", filesystem.New(filesystem.Config{
		Root:   http.Dir(staticDir),
		Browse: false,
	}))

	// Create API group with auth middleware
	apiGroup := app.Group("/api", middleware.AuthOrToken(authConfig))

	// Client routes
	apiGroup.Post("/learn-clients/register", handlers.RegisterClient)
	apiGroup.Post("/learn-clients/heartbeat", handlers.Heartbeat)
	apiGroup.Get("/learn-clients", handlers.GetClients)
	apiGroup.Get("/job", handlers.GetJob)
	apiGroup.Post("/job/result", handlers.SubmitJobResult)

	// Evaluation routes
	apiGroup.Post("/evaluations", handlers.SubmitEvaluations)
	apiGroup.Post("/positions/lookup", handlers.LookupPositions)
	apiGroup.Get("/stats/book", handlers.GetBookStats)

	// HTML routes with basic auth
	htmlGroup := app.Group("", middleware.BasicAuth(authConfig))
	htmlGroup.Get("/", htmlHandler.ShowClients)
	htmlGroup.Get("/book", htmlHandler.ShowBook)

	// Start server
	log.Fatal(app.Listen("0.0.0.0:4444"))
}
