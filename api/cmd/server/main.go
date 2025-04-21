package main

import (
	"log"

	"github.com/lk16/flippy/api/internal/app"
	"github.com/lk16/flippy/api/internal/config"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Build app
	app := app.BuildApp(cfg)

	// Start server
	address := cfg.ServerHost + ":" + cfg.ServerPort
	log.Fatal(app.Listen(address))
}
