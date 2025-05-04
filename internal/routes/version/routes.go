package version

import (
	"os/exec"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/lk16/flippy/api/internal/models"
)

var response models.VersionResponse //nolint:gochecknoglobals

// SetupRoutes sets up the routes for the version.
func SetupRoutes(app *fiber.App) {
	versionGroup := app.Group("/version")
	versionGroup.Get("/", handler)
}

// handler returns the version of the application.
func handler(c *fiber.Ctx) error {
	if response.Commit == "" {
		output, err := exec.Command("git", "rev-parse", "HEAD").Output()
		if err != nil {
			response.Commit = "unknown"
		}
		response.Commit = strings.TrimSpace(string(output))
	}

	return c.JSON(response)
}
