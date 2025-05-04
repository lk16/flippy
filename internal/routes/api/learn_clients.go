package api

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
)

// RegisterClient handles client registration.
func RegisterClient(c *fiber.Ctx) error {
	var req models.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	repo := repository.NewClientRepository(c)
	resp, err := repo.RegisterClient(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// lookupClientInRedis checks if the client ID is registered.
func lookupClientInRedis(c *fiber.Ctx) (string, error) {
	clientID := c.Get("x-client-id")
	if clientID == "" {
		return "", errors.New("missing client ID")
	}

	repo := repository.NewClientRepository(c)
	if _, err := repo.GetClientStats(c.Context(), clientID); err != nil {
		return "", err
	}

	return clientID, nil
}

// Heartbeat handles client heartbeat updates.
func Heartbeat(c *fiber.Ctx) error {
	clientID, err := lookupClientInRedis(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	repo := repository.NewClientRepository(c)
	if err = repo.UpdateHeartbeat(c.Context(), clientID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

// GetClients returns statistics for all clients.
func GetClients(c *fiber.Ctx) error {
	repo := repository.NewClientRepository(c)
	stats, err := repo.GetClientStatsList(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(stats)
}

// GetJob handles job assignment to clients.
func GetJob(c *fiber.Ctx) error {
	clientID, err := lookupClientInRedis(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	repo := repository.NewEvaluationRepository(c)
	job, err := repo.GetJob(c.Context(), clientID)

	if errors.Is(err, repository.ErrNoJobsAvailable) {
		return c.Status(fiber.StatusOK).JSON(nil)
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(job)
}
