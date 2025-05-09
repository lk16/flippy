package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/lk16/flippy/api/internal/config"
)

// BasicAuth middleware that checks for basic auth credentials.
func BasicAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		cfg := c.Locals("config").(*config.ServerConfig) //nolint: errcheck

		username := cfg.BasicAuthUsername
		password := cfg.BasicAuthPassword

		unauthorizedHandler := func(c *fiber.Ctx) error {
			// This triggers the browser to show a login dialog
			c.Set("WWW-Authenticate", `Basic realm="Restricted"`)

			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		handler := basicauth.New(basicauth.Config{
			Users: map[string]string{
				username: password,
			},
			Realm:        "Restricted",
			Unauthorized: unauthorizedHandler,
		})

		return handler(c)
	}
}

// AuthOrToken middleware that accepts either basic auth or a token header.
func AuthOrToken() fiber.Handler {
	return func(c *fiber.Ctx) error {
		cfg := c.Locals("config").(*config.ServerConfig) //nolint: errcheck

		// Check for token header first
		token := c.Get("x-token")
		if token != "" && token == cfg.Token {
			return c.Next()
		}

		// If no valid token, try basic auth
		return BasicAuth()(c)
	}
}
