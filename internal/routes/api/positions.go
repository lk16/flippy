package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
)

// LookupPositions handles position lookup requests.
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

// SubmitEvaluations handles submission of evaluation results.
func SubmitEvaluations(c *fiber.Ctx) error {
	var payload models.EvaluationsPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := payload.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	repo := repository.NewEvaluationRepository(c)
	if err := repo.SubmitEvaluations(c.Context(), payload); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

// GetBookStats returns statistics about the book.
func GetBookStats(c *fiber.Ctx) error {
	repo := repository.NewEvaluationRepository(c)
	stats, err := repo.GetBookStats(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(stats)
}
