package main

import (
	"flag"

	"github.com/lk16/flippy/api/internal/book"
	"github.com/lk16/flippy/api/internal/config"
)

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.Parse()

	cfg := config.LoadLearnClientConfig()
	learnClient := book.NewLearnClient(cfg, verbose)
	learnClient.Run()
}
