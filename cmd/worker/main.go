package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/lk16/flippy/internal/edax"
	"github.com/lk16/flippy/internal/env"
	"github.com/lk16/flippy/internal/worker"
)

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s environment variable is not set", name)
	}
	return value
}

// gitCommit best-effort determines the commit this binary was built from,
// for display on the API's worker listing. It's purely informational, so a
// deployed binary without a .git directory falls back to "unknown" rather
// than failing to start.
func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func main() {
	if err := env.Load(); err != nil {
		log.Fatalf("failed to load .env: %v", err)
	}

	serverURL := requiredEnv("FLIPPY_SERVER_URL")

	edaxPath, err := edax.PathFromEnv()
	if err != nil {
		log.Fatalf("%v", err)
	}

	workerID, err := worker.NewID()
	if err != nil {
		log.Fatalf("failed to generate worker id: %v", err)
	}
	log.Printf("worker id: %s", workerID)

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	edaxProcess := edax.NewProcess(edaxPath)
	client := worker.NewClient(serverURL, workerID, hostname, gitCommit())
	w := worker.New(client, edaxProcess)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Canceling ctx alone can't interrupt a blocked edax evaluation — edax's
	// process I/O doesn't take a context — so shutdown also closes the
	// process directly, which unblocks it with an error.
	go func() {
		<-ctx.Done()
		_ = edaxProcess.Close()
	}()

	w.Run(ctx)

	if err := edaxProcess.Close(); err != nil {
		log.Printf("error closing edax process: %v", err)
	}
}
