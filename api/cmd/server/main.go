package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/lk16/flippy/api/internal/db"
	"github.com/lk16/flippy/api/internal/handlers"
	"github.com/lk16/flippy/api/internal/middleware"
	"github.com/lk16/flippy/api/internal/repository"
)

func main() {
	// Initialize database
	if err := db.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize Redis
	if err := db.InitRedis(); err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}

	// Create repositories
	clientRepo := repository.NewClientRepository(db.GetRedis())
	evalRepo := repository.NewEvaluationRepository(clientRepo, db.GetRedis())

	// Get the path to the static files
	staticDir := "../src/flippy/book/static"

	// Create handlers
	clientHandler := handlers.NewClientHandler(clientRepo, db.GetRedis())
	evalHandler := handlers.NewEvaluationHandler(evalRepo)
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
	apiGroup.Post("/learn-clients/register", clientHandler.RegisterClient)
	apiGroup.Post("/learn-clients/heartbeat", clientHandler.Heartbeat)
	apiGroup.Get("/learn-clients", clientHandler.GetClients)
	apiGroup.Get("/job", clientHandler.GetJob)
	apiGroup.Post("/job/result", clientHandler.SubmitJobResult)

	// Evaluation routes
	apiGroup.Post("/evaluations", evalHandler.SubmitEvaluations)
	apiGroup.Post("/positions/lookup", evalHandler.LookupPositions)
	apiGroup.Get("/stats/book", evalHandler.GetBookStats)

	// HTML routes with basic auth
	htmlGroup := app.Group("", middleware.BasicAuth(authConfig))
	htmlGroup.Get("/", htmlHandler.ShowClients)
	htmlGroup.Get("/book", htmlHandler.ShowBook)

	// Start server
	log.Fatal(app.Listen("0.0.0.0:4444"))
}
