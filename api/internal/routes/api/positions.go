package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
)

// LookupPositions handles position lookup requests
func LookupPositions(c *fiber.Ctx) error {
	var payload models.LookupPositionsPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	repo := repository.NewEvaluationRepository(c)
	evaluations, err := repo.LookupPositions(c.Context(), payload.Positions)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(evaluations)
}
