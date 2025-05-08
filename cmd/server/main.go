package main

import (
	"log"

	"github.com/lk16/flippy/api/internal"
)

func main() {
	// Setup app
	app, cfg := internal.SetupApp()

	// Start server
	address := cfg.ServerHost + ":" + cfg.ServerPort
	log.Fatal(app.Listen(address))
}
