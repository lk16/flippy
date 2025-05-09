package main

import (
	"log"

	"github.com/lk16/flippy/api/internal"
	"github.com/lk16/flippy/api/internal/config"
)

func main() {
	// Set log level
	config.SetLogLevel()

	// Setup app
	app, cfg := internal.SetupApp()

	// Start server
	address := cfg.ServerHost + ":" + cfg.ServerPort
	log.Fatal(app.Listen(address))
}
