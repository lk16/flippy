package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/env"
	"github.com/lk16/flippy/internal/loader"
)

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s environment variable is not set", name)
	}
	return value
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: loader <command>")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  seed    add the precomputed 12-disc board set to the DB")
	os.Exit(1)
}

func main() {
	if err := env.Load(); err != nil {
		log.Fatalf("failed to load .env: %v", err)
	}

	if len(os.Args) < 2 {
		usage()
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, requiredEnv("FLIPPY_POSTGRES_URL"))
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	repo := db.NewRepository(pool)

	switch os.Args[1] {
	case "seed":
		if err := loader.SeedBoards(ctx, repo); err != nil {
			log.Fatalf("failed to seed boards: %v", err)
		}
		log.Println("seeded the precomputed 12-disc board set (existing rows left untouched)")
	default:
		usage()
	}
}
