package tests

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/app"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/services"
	"github.com/testcontainers/testcontainers-go"
)

func projectRoot() string {

	dir, err := os.Getwd()
	if err != nil {
		panic("failed to get project root: " + err.Error())
	}
	return filepath.Clean(dir + "/../../..")
}

type TestContainers struct {
	Postgres testcontainers.Container
	Redis    testcontainers.Container
}

func (tc *TestContainers) Start(ctx context.Context, cfg config.Config) error {
	// Start PostgreSQL container
	postgresReq := testcontainers.ContainerRequest{
		Image:        "postgres:latest",
		Name:         "test-postgres",
		ExposedPorts: []string{"5433:5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "pg-test-user",
			"POSTGRES_PASSWORD": "pg-test-password",
			"POSTGRES_DB":       "pg-test-db",
		},
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.AutoRemove = true
		},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      projectRoot() + "/schema.sql",
				ContainerFilePath: "/docker-entrypoint-initdb.d/init.sql",
				FileMode:          0644,
			},
			{
				HostFilePath:      projectRoot() + "/api/test_data.sql",
				ContainerFilePath: "/test_data.sql",
				FileMode:          0644,
			},
		},
	}

	var err error
	tc.Postgres, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
	})
	if err != nil {
		return err
	}

	// Wait for PostgreSQL to be ready
	pgOk := false
	for i := 0; i < 10; i++ {
		_, err := services.InitPostgres(cfg.PostgresURL)
		if err != nil {
			log.Printf("failed to connect to postgres: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("postgres is ready")
		pgOk = true
		break
	}

	if !pgOk {
		return fmt.Errorf("postgres took too long to start")
	}

	status, reader, err := tc.Postgres.Exec(ctx, []string{"psql", "-U", "pg-test-user", "-d", "pg-test-db", "-f", "/test_data.sql"})
	if err != nil || status != 0 {
		read, err := io.ReadAll(reader)
		if err != nil {
			panic(err)
		}

		log.Printf("reader: %s", string(read))
		return fmt.Errorf("failed to execute test_data.sql (status: %d): %w", status, err)
	}

	// Start Redis container
	redisReq := testcontainers.ContainerRequest{
		Image:        "redis:7",
		Name:         "test-redis",
		ExposedPorts: []string{"6380:6379/tcp"},
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			hostConfig.AutoRemove = true
		},
	}

	tc.Redis, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: redisReq,
		Started:          true,
	})
	if err != nil {
		return err
	}

	// Wait for Redis to be ready
	redisOk := false
	for i := 0; i < 10; i++ {
		_, err := services.InitRedis(cfg.RedisURL)

		if err != nil {
			log.Printf("failed to connect to redis: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("redis is ready")
		redisOk = true
		break
	}

	if !redisOk {
		return fmt.Errorf("redis took too long to start")
	}

	return nil
}

func (tc *TestContainers) Stop(ctx context.Context) error {
	if tc.Postgres != nil {
		if err := tc.Postgres.Terminate(ctx); err != nil {
			return err
		}
	}
	if tc.Redis != nil {
		if err := tc.Redis.Terminate(ctx); err != nil {
			return err
		}
	}
	return nil
}

func StartApplication(cfg config.Config) *fiber.App {

	app := app.BuildApp(&cfg)

	go func() {
		if err := app.Listen(cfg.ServerHost + ":" + cfg.ServerPort); err != nil {
			panic(err)
		}
	}()

	return app
}

func WaitForApplication(app *fiber.App, cfg config.Config) {
	appOk := false
	for i := 0; i < 10; i++ {
		_, err := http.Get("http://" + cfg.ServerHost + ":" + cfg.ServerPort + "/")
		if err != nil {
			log.Printf("failed to get health: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Printf("application is ready")
		appOk = true
		break
	}

	if !appOk {
		panic("application took too long to start")
	}
}

func StopApplication(app *fiber.App) {
	if err := app.Shutdown(); err != nil {
		panic(err)
	}
}

func TestMain(m *testing.M) {
	cfg := config.Config{
		ServerHost:        "localhost",
		ServerPort:        "3000",
		RedisURL:          "redis://localhost:6380",
		PostgresURL:       "postgres://pg-test-user:pg-test-password@localhost:5433/pg-test-db?sslmode=disable",
		BasicAuthUsername: "pg-test-user",
		BasicAuthPassword: "pg-test-password",
		Token:             "pg-test-token",
		Prefork:           false,
	}

	ctx := context.Background()
	containers := &TestContainers{}

	if err := containers.Start(ctx, cfg); err != nil {
		panic(err)
	}

	defer func() {
		if err := containers.Stop(ctx); err != nil {
			panic(err)
		}
	}()

	app := StartApplication(cfg)

	WaitForApplication(app, cfg)

	defer StopApplication(app)

	os.Exit(m.Run())
}
