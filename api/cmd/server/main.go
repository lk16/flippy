package main

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/handlers"
	"github.com/lk16/flippy/api/internal/middleware"
	"github.com/lk16/flippy/api/internal/services"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		Prefork:      true,
		Concurrency:  256 * 1024, // Maximum number of concurrent connections per worker
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  5 * time.Second,
		BodyLimit:    1024 * 1024, // 1MB
	})

	// Initialize services
	services, err := services.InitServices(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	// Setup connections to external services and config in Fiber app
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("services", services)
		c.Locals("config", cfg)
		return c.Next()
	})

	// Add logging middleware
	app.Use(middleware.Logging())

	// Add CORS middleware
	app.Use(cors.New())

	// Create API group with auth middleware
	apiGroup := app.Group("/api", middleware.AuthOrToken())

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

	// Serve static files
	app.Use("/static", handlers.StaticHandler())

	// HTML routes with basic auth
	htmlGroup := app.Group("", middleware.BasicAuth())
	htmlGroup.Get("/", handlers.ClientsPage)
	htmlGroup.Get("/book", handlers.BookPage)

	// Start server
	address := cfg.ServerHost + ":" + cfg.ServerPort
	log.Fatal(app.Listen(address))
}
