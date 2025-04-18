package main

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/middleware"
	"github.com/lk16/flippy/api/internal/repository"
	"github.com/lk16/flippy/api/internal/routes"
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

	// (Re)build book stats, before starting the server
	evaluationRepo := repository.NewEvaluationRepositoryFromServices(services)
	err = evaluationRepo.RefreshBookStats(context.Background())
	if err != nil {
		log.Fatalf("Failed to refresh book stats: %v", err)
	}

	// Setup connections to external services and config in Fiber app
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("services", services)
		c.Locals("config", cfg)
		return c.Next()
	})

	// Add logging middleware
	app.Use(middleware.Logging())

	// Setup all routes
	routes.SetupRoutes(app)

	// Start server
	address := cfg.ServerHost + ":" + cfg.ServerPort
	log.Fatal(app.Listen(address))
}
