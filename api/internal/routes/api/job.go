package api

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
)

// GetJob handles job assignment to clients
func GetJob(c *fiber.Ctx) error {
	clientID, err := GetClient(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	repo := repository.NewEvaluationRepository(c)
	job, err := repo.GetJob(c.Context(), clientID)

	if err == repository.ErrNoJobsAvailable {
		return c.Status(fiber.StatusOK).JSON(nil)
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(job)
}

// SubmitJobResult handles job result submission
func SubmitJobResult(c *fiber.Ctx) error {
	clientID, err := GetClient(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var result models.JobResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Invalid request body: %s", err.Error()),
		})
	}

	evalRepo := repository.NewEvaluationRepository(c)

	payload := models.EvaluationsPayload{
		Evaluations: []models.Evaluation{result.Evaluation},
	}

	if err := evalRepo.SubmitEvaluations(c.Context(), payload); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Refresh stats view in background
	go evalRepo.RefreshStatsView(c.Context())

	// Mark job as completed
	repo := repository.NewClientRepository(c)
	if err := repo.CompleteJob(c.Context(), clientID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}
