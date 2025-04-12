package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
)

// SubmitEvaluations handles submission of evaluation results
func SubmitEvaluations(c *fiber.Ctx) error {
	var payload models.EvaluationsPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	repo := repository.NewEvaluationRepository(c)
	if err := repo.SubmitEvaluations(c.Context(), payload); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Refresh stats view in background
	go repo.RefreshStatsView(c.Context())

	return c.SendStatus(fiber.StatusOK)
}
