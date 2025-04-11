package middleware

import (
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
)

// BasicAuthConfig holds the configuration for basic auth
type BasicAuthConfig struct {
	Username string
	Password string
}

// NewBasicAuthConfig creates a new basic auth config from environment variables
func NewBasicAuthConfig() *BasicAuthConfig {
	return &BasicAuthConfig{
		Username: os.Getenv("FLIPPY_BOOK_SERVER_BASIC_AUTH_USER"),
		Password: os.Getenv("FLIPPY_BOOK_SERVER_BASIC_AUTH_PASS"),
	}
}

// BasicAuth middleware that checks for basic auth credentials
func BasicAuth(config *BasicAuthConfig) fiber.Handler {
	unauthorizedHandler := func(c *fiber.Ctx) error {
		// This triggers the browser to show a login dialog
		c.Set("WWW-Authenticate", `Basic realm="Restricted"`)

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	return basicauth.New(basicauth.Config{
		Users: map[string]string{
			config.Username: config.Password,
		},
		Realm:        "Restricted",
		Unauthorized: unauthorizedHandler,
	})
}

// AuthOrToken middleware that accepts either basic auth or a token header
func AuthOrToken(config *BasicAuthConfig) fiber.Handler {
	// Get the expected token from environment
	expectedToken := os.Getenv("FLIPPY_BOOK_SERVER_TOKEN")
	basicAuth := BasicAuth(config)

	return func(c *fiber.Ctx) error {
		// Skip auth for /api/learn-clients/register
		if c.Path() == "/api/learn-clients/register" {
			return c.Next()
		}

		// Check for token header first
		token := c.Get("x-token")
		if token != "" && token == expectedToken {
			return c.Next()
		}

		// If no valid token, try basic auth
		return basicAuth(c)
	}
}
