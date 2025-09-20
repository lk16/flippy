package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/lk16/flippy/api/internal/config"
	"github.com/lk16/flippy/api/internal/middleware"
)

func SetupRoutes(app *fiber.App) {
	apiGroup := app.Group("/api", middleware.AuthOrToken())
	apiGroup.Post("/learn-clients/register", RegisterClient)
	apiGroup.Post("/learn-clients/heartbeat", Heartbeat)
	apiGroup.Get("/learn-clients", GetClients)
	apiGroup.Get("/learn-clients/job", GetJob)
	apiGroup.Post("/positions/evaluations", SubmitEvaluations)
	apiGroup.Post("/positions/lookup", LookupPositions)
	apiGroup.Get("/positions/stats", GetBookStats)

	// Serve static files
	app.Use("/static", staticHandler())

	// Serve HTML pages
	bookGroup := app.Group("/book", middleware.BasicAuth())
	bookGroup.Get("/", BookPage)

	clientsGroup := app.Group("/clients", middleware.BasicAuth())
	clientsGroup.Get("/", ClientsPage)

	// Serve version info
	versionGroup := app.Group("/version")
	versionGroup.Get("/", Handler)

	// Serve websocket
	app.Get("/ws", websocket.New(HandleWs))

	// Serve gamepage
	app.Get("/game", GamePage)

	// Serve root page
	app.Get("/", rootHandler)
}

func rootHandler(c *fiber.Ctx) error {
	return c.Redirect("/game")
}

// staticHandler serves static files.
func staticHandler() fiber.Handler {
	cfg := config.LoadServerConfig()

	return filesystem.New(filesystem.Config{
		Root:   http.Dir(cfg.StaticDir),
		Browse: false,
	})
}

func getServices(c *fiber.Ctx) *Services {
	return c.Locals("services").(*Services) // nolint:errcheck
}

func getConfig(c *fiber.Ctx) *config.ServerConfig {
	return c.Locals("config").(*config.ServerConfig) // nolint:errcheck
}

// BookPage serves the book.html page.
func BookPage(c *fiber.Ctx) error {
	staticDir := getConfig(c).StaticDir
	return c.SendFile(filepath.Join(staticDir, "book.html"))
}

// ClientsPage serves the clients.html page.
func ClientsPage(c *fiber.Ctx) error {
	staticDir := getConfig(c).StaticDir
	return c.SendFile(filepath.Join(staticDir, "clients.html"))
}

// GamePage serves the game.html page.
func GamePage(c *fiber.Ctx) error {
	staticDir := getConfig(c).StaticDir
	return c.SendFile(filepath.Join(staticDir, "game.html"))
}

// RegisterClient handles client registration.
func RegisterClient(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	resp, err := registerClient(c.Context(), getServices(c), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// lookupClientInRedis checks if the client ID is registered.
func lookupClientInRedis(c *fiber.Ctx) (string, error) {
	clientID := c.Get("x-client-id")
	if clientID == "" {
		return "", errors.New("missing client ID")
	}

	if _, err := getClientStats(c.Context(), getServices(c), clientID); err != nil {
		return "", err
	}

	return clientID, nil
}

// Heartbeat handles client heartbeat updates.
func Heartbeat(c *fiber.Ctx) error {
	clientID, err := lookupClientInRedis(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err = updateHeartbeat(c.Context(), getServices(c), clientID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

// GetClients returns statistics for all clients.
func GetClients(c *fiber.Ctx) error {
	stats, err := getClientStatsList(c.Context(), getServices(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(stats)
}

// GetJob handles job assignment to clients.
func GetJob(c *fiber.Ctx) error {
	clientID, err := lookupClientInRedis(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	job, err := getJob(c.Context(), getServices(c), clientID)

	if errors.Is(err, ErrNoJobsAvailable) {
		return c.Status(fiber.StatusOK).JSON(nil)
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(job)
}

// LookupPositions handles position lookup requests.
func LookupPositions(c *fiber.Ctx) error {
	var payload LookupPositionsPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	evaluations, err := lookupPositions(c.Context(), getServices(c), payload.Positions)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(evaluations)
}

// SubmitEvaluations handles submission of evaluation results.
func SubmitEvaluations(c *fiber.Ctx) error {
	var payload EvaluationsPayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := payload.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := submitEvaluations(c.Context(), getServices(c), payload); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

// GetBookStats returns statistics about the book.
func GetBookStats(c *fiber.Ctx) error {
	stats, err := getBookStats(c.Context(), getServices(c))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(stats)
}

var response VersionResponse

// Handler returns the version of the application.
func Handler(c *fiber.Ctx) error {
	if response.Commit == "" {
		output, err := exec.Command("git", "rev-parse", "HEAD").Output()
		if err != nil {
			response.Commit = "unknown"
		}
		response.Commit = strings.TrimSpace(string(output))
	}

	return c.JSON(response)
}

func HandleWs(c *websocket.Conn) {
	services := c.Locals("services").(*Services) //nolint: errcheck

	h := NewWsHandler(c, services)
	err := h.Handle()
	if err != nil {
		slog.Error("ws handle error", "error", err)
	}
}

const (
	wsEvaluationLoadTimeout = 2 * time.Second
)

type WsHandler struct {
	services *Services
	ws       *websocket.Conn
}

// NewWsHandler creates a new Handler.
func NewWsHandler(ws *websocket.Conn, services *Services) *WsHandler {
	return &WsHandler{services: services, ws: ws}
}

var errClientDisconnected = errors.New("client disconnected")

func (h *WsHandler) readMessage() (*Incoming, error) {
	var req Incoming

	msgType, msg, err := h.ws.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(err, websocket.CloseGoingAway) {
			return nil, errClientDisconnected
		}

		return nil, fmt.Errorf("ws read error: %w", err)
	}

	slog.Debug("read ws message", "msgType", msgType, "msg", msg)

	if msgType != websocket.TextMessage {
		return nil, fmt.Errorf("unexpected message type: %d", msgType)
	}

	if err = json.Unmarshal(msg, &req); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &req, nil
}

func (h *WsHandler) writeMessage(outgoing *Outgoing) error {
	msg, err := json.Marshal(outgoing)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	slog.Debug("write ws message", "msg", string(msg))

	if err = h.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	return nil
}

func (h *WsHandler) handleMessage(req *Incoming) (*Outgoing, error) {
	if req.Event == "" {
		return nil, errors.New("event field is either empty or missing")
	}

	switch req.Event {
	case "evaluation_request":
		return h.handleEvaluationRequest(req)
	default:
		return nil, fmt.Errorf("unknown event: %s", req.Event)
	}
}

// Handle handles the websocket connection.
func (h *WsHandler) Handle() error {
	for {
		req, err := h.readMessage()
		if err != nil {
			if errors.Is(err, errClientDisconnected) {
				return nil
			}

			return fmt.Errorf("ws read error: %w", err)
		}

		respData, err := h.handleMessage(req)
		if err != nil {
			return fmt.Errorf("ws handle error: %w", err)
		}

		if err = h.writeMessage(respData); err != nil {
			return fmt.Errorf("ws write error: %w", err)
		}
	}
}

func (h *WsHandler) handleEvaluationRequest(req *Incoming) (*Outgoing, error) {
	var reqData EvaluationRequest
	if err := json.Unmarshal(req.Data, &reqData); err != nil {
		return nil, fmt.Errorf("ws evaluation request unmarshal error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), wsEvaluationLoadTimeout)
	defer cancel()

	evaluations, err := lookupPositions(ctx, h.services, reqData.Positions)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup positions: %w", err)
	}

	outgoing := &Outgoing{
		ID: req.ID,
		Data: EvaluationResponse{
			Evaluations: evaluations,
		},
	}

	return outgoing, nil
}
