package main

import (
	"log"

	"github.com/lk16/flippy/api/internal/book"
	"github.com/lk16/flippy/api/internal/config"
)

func main() {
	config.SetLogLevel()

	cfg := config.LoadLearnClientConfig()

	learnClient, err := book.NewLearnClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create learn client: %v", err)
	}

	learnClient.Run()
}
