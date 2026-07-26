package main

import (
	"fmt"
	"log"

	"github.com/lk16/flippy/internal/env"
)

func main() {
	if err := env.Load(); err != nil {
		log.Fatalf("failed to load .env: %v", err)
	}

	fmt.Println("flippy loader")
}
