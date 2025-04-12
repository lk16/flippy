package handlers

import (
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

// TODO remove this handler struct
type HTMLHandler struct {
	staticDir string
}

func NewHTMLHandler(staticDir string) *HTMLHandler {
	return &HTMLHandler{
		staticDir: staticDir,
	}
}

// ShowClients serves the clients.html page
func (h *HTMLHandler) ShowClients(c *fiber.Ctx) error {
	return c.SendFile(filepath.Join(h.staticDir, "clients.html"))
}

// ShowBook serves the book.html page
func (h *HTMLHandler) ShowBook(c *fiber.Ctx) error {
	return c.SendFile(filepath.Join(h.staticDir, "book.html"))
}
