package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/repository"
)

// GetBookStats returns statistics about the book
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
