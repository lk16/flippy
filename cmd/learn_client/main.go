package main

import (
	"os"

	"log"

	"github.com/lk16/flippy/api/internal/cmd/learn"
	"github.com/lk16/flippy/api/internal/config"
)

func main() {
	config.SetLogLevel()

	cfg := config.LoadLearnClientConfig()

	learnClient, err := learn.NewClient(cfg)
	if err != nil {
		log.Printf("Failed to create learn client: %v", err)
		os.Exit(1)
	}

	learnClient.Run()
}
