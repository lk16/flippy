package main

import (
	"flag"
	"log"

	"github.com/lk16/flippy/api/internal/gui"
	"github.com/lk16/flippy/api/internal/othello"
)

func main() {
	defaultStart := othello.NewBoardStart().String()
	start := flag.String("start", defaultStart, "the start position")
	flag.Parse()

	startBoard, err := othello.NewBoardFromString(*start)
	if err != nil {
		log.Fatalf("failed to create board: %v", err)
	}

	window, err := gui.NewWindow(startBoard)
	if err != nil {
		log.Fatalf("failed to create window: %v", err)
	}

	window.Run()
}
