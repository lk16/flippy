package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/repository"
)

type ClientHandler struct {
	clientRepo *repository.ClientRepository
}

func NewClientHandler(clientRepo *repository.ClientRepository) *ClientHandler {
	return &ClientHandler{
		clientRepo: clientRepo,
	}
}

// RegisterClient handles client registration
func (h *ClientHandler) RegisterClient(c *fiber.Ctx) error {
	var req models.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	resp, err := h.clientRepo.RegisterClient(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// Heartbeat handles client heartbeat updates
func (h *ClientHandler) Heartbeat(c *fiber.Ctx) error {
	// TODO move this and similar into middleware
	clientID := c.Get("client-id")
	if clientID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing client ID",
		})
	}

	if err := h.clientRepo.UpdateHeartbeat(c.Context(), clientID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

// GetClients returns statistics for all clients
func (h *ClientHandler) GetClients(c *fiber.Ctx) error {
	stats, err := h.clientRepo.GetClientStats(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(stats)
}

// GetJob handles job assignment to clients
func (h *ClientHandler) GetJob(c *fiber.Ctx) error {
	clientID := c.Get("client-id")
	if clientID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing client ID",
		})
	}

	// Prune inactive clients first
	if err := h.clientRepo.PruneInactiveClients(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	repo := repository.NewEvaluationRepository()
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
func (h *ClientHandler) SubmitJobResult(c *fiber.Ctx) error {
	clientID := c.Get("client-id")
	if clientID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing client ID",
		})
	}

	var result models.JobResult
	if err := c.BodyParser(&result); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Invalid request body: %s", err.Error()),
		})
	}

	evalRepo := repository.NewEvaluationRepository()

	payload := models.EvaluationsPayload{
		Evaluations: []models.SerializedEvaluation{result.Evaluation},
	}

	if err := evalRepo.SubmitEvaluations(c.Context(), payload); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Refresh stats view in background
	go evalRepo.RefreshStatsView(c.Context())

	// Mark job as completed
	if err := h.clientRepo.CompleteJob(c.Context(), clientID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}
