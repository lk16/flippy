package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lk16/flippy/internal/db"
	"github.com/lk16/flippy/internal/env"
	"github.com/lk16/flippy/internal/loader"
)

// progressReportInterval caps log lines to one per interval so a large import doesn't spam a line per file.
const progressReportInterval = 2 * time.Second

// logProgress returns a loader.ImportPaths progress callback that logs completion percentage and ETA.
func logProgress() func(done, total int) {
	start := time.Now()
	var last time.Time

	return func(done, total int) {
		if total == 0 {
			return
		}

		now := time.Now()
		finished := done == total
		if !finished && now.Sub(last) < progressReportInterval {
			return
		}
		last = now

		elapsed := now.Sub(start)

		var eta time.Duration
		if done > 0 {
			eta = time.Duration(float64(elapsed) / float64(done) * float64(total-done))
		}

		log.Printf("parsed %d/%d files (%.1f%%), elapsed %s, ETA %s",
			done, total, 100*float64(done)/float64(total),
			elapsed.Round(time.Second), eta.Round(time.Second))
	}
}

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
	fmt.Fprintln(os.Stderr, "  seed                       add the precomputed 12-disc board set to the DB")
	fmt.Fprintln(os.Stderr, "  load <files/folders...>    import boards from .wtb/.pgn files, searching folders recursively")
	fmt.Fprintln(os.Stderr, "  load-oq <moves>            import boards from an Othello Quest move string")
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
		if err := loader.SeedPositions(ctx, repo); err != nil {
			log.Fatalf("failed to seed boards: %v", err)
		}
		log.Println("seeded the precomputed 12-disc board set (existing rows left untouched)")
	case "load":
		paths := requireFiles("load")
		count, err := loader.ImportPaths(ctx, repo, paths, logProgress())
		if err != nil {
			log.Fatalf("failed to import: %v", err)
		}
		log.Printf("added %d boards from %d path(s)", count, len(paths))
	case "load-oq":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: loader load-oq <moves>")
			os.Exit(1)
		}
		count, err := loader.ImportOthelloQuestMoves(ctx, repo, os.Args[2])
		if err != nil {
			log.Fatalf("failed to import move string: %v", err)
		}
		log.Printf("added %d boards", count)
	default:
		usage()
	}
}

// requireFiles returns the filenames passed after command, exiting with usage if none were given.
func requireFiles(command string) []string {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: loader %s <files...>\n", command)
		os.Exit(1)
	}
	return os.Args[2:]
}
