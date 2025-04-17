package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/constants"
	"github.com/lk16/flippy/api/internal/middleware"
	"github.com/lk16/flippy/api/internal/repository"
	"github.com/lk16/flippy/api/internal/routes"
	"github.com/lk16/flippy/api/internal/services"
)

// OnServerStart is called when the server starts up
func OnServerStart(s *services.Services) error {
	ctx := context.Background()

	evaluationRepo := repository.NewEvaluationRepositoryFromServices(s)

	// Refresh book stats
	if err := evaluationRepo.RefreshBookStats(ctx); err != nil {
		return fmt.Errorf("error refreshing book stats: %w", err)
	}

	// Get remaining job count
	remainingJobCount, err := s.Redis.LLen(ctx, constants.PositionsKey).Result()
	if err != nil {
		return fmt.Errorf("error getting remaining job count: %w", err)
	}

	// Refresh jobs list if it's running low
	if remainingJobCount < constants.RefillThreshold {
		if err := evaluationRepo.RefillJobCache(ctx); err != nil {
			return fmt.Errorf("error refreshing jobs list: %w", err)
		}
	}

	return nil
}

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

	// Connect to services
	services, err := services.GetServices(cfg)
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

	// Setup all routes
	routes.SetupRoutes(app)

	// Call hooks to initialize services
	if err := OnServerStart(services); err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	// Start server
	address := cfg.ServerHost + ":" + cfg.ServerPort
	log.Fatal(app.Listen(address))
}
