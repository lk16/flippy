package main

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/middleware"
	"github.com/lk16/flippy/api/internal/routes"
	"github.com/lk16/flippy/api/internal/services"
)

const (
	defaultConcurrency  = 256 * 1024 // Maximum number of concurrent connections per worker
	defaultReadTimeout  = 10 * time.Second
	defaultWriteTimeout = 10 * time.Second
	defaultIdleTimeout  = 5 * time.Second
	defaultBodyLimit    = 1024 * 1024 // 1MB
)

func main() {
	// Load configuration
	cfg := config.LoadServerConfig()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		Prefork:      cfg.Prefork,
		Concurrency:  defaultConcurrency,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		IdleTimeout:  defaultIdleTimeout,
		BodyLimit:    defaultBodyLimit,
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

	// Setup all routes
	routes.SetupRoutes(app)

	// Start server
	address := cfg.ServerHost + ":" + cfg.ServerPort
	log.Fatal(app.Listen(address))
}
