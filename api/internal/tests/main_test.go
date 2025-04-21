package tests

import (
	"context"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/app"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestContainers struct {
	Postgres testcontainers.Container
	Redis    testcontainers.Container
}

func (tc *TestContainers) Start(ctx context.Context) error {
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
		WaitingFor: wait.ForLog("database system is ready to accept connections"),
	}

	var err error
	tc.Postgres, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
	})
	if err != nil {
		return err
	}

	// Start Redis container
	redisReq := testcontainers.ContainerRequest{
		Image:        "redis:7",
		Name:         "test-redis",
		ExposedPorts: []string{"6380:6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}

	tc.Redis, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: redisReq,
		Started:          true,
	})
	if err != nil {
		return err
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

func StartApplication() *fiber.App {
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

	app := app.BuildApp(&cfg)

	go func() {
		if err := app.Listen(cfg.ServerHost + ":" + cfg.ServerPort); err != nil {
			panic(err)
		}
	}()

	return app
}

func StopApplication(app *fiber.App) {
	if err := app.Shutdown(); err != nil {
		panic(err)
	}
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	containers := &TestContainers{}

	if err := containers.Start(ctx); err != nil {
		panic(err)
	}

	defer func() {
		if err := containers.Stop(ctx); err != nil {
			panic(err)
		}
	}()

	// TODO remove
	select {}

	app := StartApplication()

	defer StopApplication(app)

	os.Exit(m.Run())
}
