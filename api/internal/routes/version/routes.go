package version

import (
	"os/exec"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type VersionResponse struct {
	Commit string `json:"commit"`
}

var Version VersionResponse

func init() {
	output, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		Version.Commit = "unknown"
	}
	Version.Commit = strings.TrimSpace(string(output))
}

func SetupRoutes(app *fiber.App) {
	versionGroup := app.Group("/version")
	versionGroup.Get("/", versionHandler)
}

func versionHandler(c *fiber.Ctx) error {
	return c.JSON(Version)
}
