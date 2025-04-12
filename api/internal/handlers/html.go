package handlers

import (
	"net/http"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
)

const StaticDir = "../src/flippy/book/static"

func StaticHandler() fiber.Handler {
	return filesystem.New(filesystem.Config{
		Root:   http.Dir(StaticDir),
		Browse: false,
	})
}

// ShowClients serves the clients.html page
func ClientsPage(c *fiber.Ctx) error {
	return c.SendFile(filepath.Join(StaticDir, "clients.html"))
}

// ShowBook serves the book.html page
func BookPage(c *fiber.Ctx) error {
	return c.SendFile(filepath.Join(StaticDir, "book.html"))
}
