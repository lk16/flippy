package main

import (
	"context"
	"flag"
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

// gitCommit best-effort determines the build commit, falling back to "unknown" without a .git directory.
func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func main() {
	edaxTasks := flag.Int("edax-tasks", 0,
		"cap edax's parallel search threads per process (its -n-tasks flag); 0 leaves it unset, so "+
			"edax defaults to one thread per CPU. Set this to run multiple workers on one machine "+
			"without them oversubscribing its CPUs.")
	flag.Parse()

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

	edaxProcess := edax.NewProcess(edaxPath, *edaxTasks)
	client := worker.NewClient(serverURL, workerID, hostname, gitCommit())
	w := worker.New(client, edaxProcess)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ctx cancellation alone can't interrupt a blocked edax evaluation, so close the process directly too.
	go func() {
		<-ctx.Done()
		_ = edaxProcess.Close()
	}()

	w.Run(ctx)

	if err := edaxProcess.Close(); err != nil {
		log.Printf("error closing edax process: %v", err)
	}
}
