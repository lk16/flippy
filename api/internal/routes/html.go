package routes

import (
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

// ClientsPage serves the clients.html page
func ClientsPage(c *fiber.Ctx) error {
	return c.SendFile(filepath.Join(StaticDir, "clients.html"))
}

// BookPage serves the book.html page
func BookPage(c *fiber.Ctx) error {
	return c.SendFile(filepath.Join(StaticDir, "book.html"))
}
