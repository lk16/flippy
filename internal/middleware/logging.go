package middleware

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

// Logging middleware that logs route, status code and response time.
func Logging() fiber.Handler {
	return logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} | ${path}\n",
		TimeFormat: "2006-01-02 15:04:05",
		TimeZone:   "Local",
		CustomTags: map[string]logger.LogFunc{
			"latency": func(output logger.Buffer, _ *fiber.Ctx, data *logger.Data, _ string) (int, error) {
				latency := float64(data.Stop.Sub(data.Start).Nanoseconds()) / float64(time.Millisecond)
				return fmt.Fprintf(output, "%6.1fms", latency)
			},
		},
	})
}
