package middleware

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// Logging middleware that logs route, status code and response time
func Logging() fiber.Handler {
	return logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} | ${path}\n",
		TimeFormat: "2006-01-02 15:04:05",
		TimeZone:   "Local",
		CustomTags: map[string]logger.LogFunc{
			"latency": func(output logger.Buffer, c *fiber.Ctx, data *logger.Data, extraParam string) (int, error) {
				latency := float64(data.Stop.Sub(data.Start).Nanoseconds()) / 1_000_000.0
				return output.WriteString(fmt.Sprintf("%6.1fms", latency))
			},
		},
	})
}
