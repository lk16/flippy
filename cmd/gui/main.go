package main

import (
	"flag"
	"log"

	"github.com/lk16/flippy/api/internal/gui"
)

func main() {
	mode := flag.String("mode", "game", "the mode to run")
	flag.Parse()

	window, err := gui.NewWindow(*mode)
	if err != nil {
		log.Fatalf("failed to create window: %v", err)
	}

	window.Run()
}
