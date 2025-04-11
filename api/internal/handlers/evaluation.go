package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
)

// EvaluationHandler handles evaluation-related HTTP requests
type EvaluationHandler struct {
	evalRepo *repository.EvaluationRepository
}

// NewEvaluationHandler creates a new EvaluationHandler
func NewEvaluationHandler(evalRepo *repository.EvaluationRepository) *EvaluationHandler {
	return &EvaluationHandler{
		evalRepo: evalRepo,
	}
}

// SubmitEvaluations handles batch evaluation submissions
func (h *EvaluationHandler) SubmitEvaluations(c *fiber.Ctx) error {
	var payload models.EvaluationsPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := h.evalRepo.SubmitEvaluations(c.Context(), payload); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Refresh stats view in background
	go h.evalRepo.RefreshStatsView(c.Context())

	return c.SendStatus(fiber.StatusOK)
}

// LookupPositions handles position lookup requests
func (h *EvaluationHandler) LookupPositions(c *fiber.Ctx) error {
	var payload models.LookupPositionsPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	evaluations, err := h.evalRepo.LookupPositions(c.Context(), payload.Positions)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(evaluations)
}

// GetBookStats returns statistics about the evaluation database
func (h *EvaluationHandler) GetBookStats(c *fiber.Ctx) error {
	stats, err := h.evalRepo.GetBookStats(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(stats)
}
