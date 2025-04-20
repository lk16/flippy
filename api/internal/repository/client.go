package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/lk16/flippy/api/internal/services"
	"github.com/redis/go-redis/v9"
)

const (
	ClientsKey = "clients"
	ClientsTTL = 300 * time.Second
)

type ClientRepository struct {
	services *services.Services
}

func NewClientRepository(c *fiber.Ctx) *ClientRepository {
	return &ClientRepository{
		services: c.Locals("services").(*services.Services),
	}
}

func NewClientRepositoryFromServices(services *services.Services) *ClientRepository {
	return &ClientRepository{
		services: services,
	}
}

// RegisterClient registers a new client and returns its ID
func (repo *ClientRepository) RegisterClient(ctx context.Context, req models.RegisterRequest) (models.RegisterResponse, error) {
	clientID := uuid.New().String()

	clientStats := models.ClientStats{
		ID:                clientID,
		Hostname:          req.Hostname,
		GitCommit:         req.GitCommit,
		PositionsComputed: 0,
		LastActive:        time.Now(),
		Position:          models.NewNormalizedPositionEmpty(),
	}

	// Convert to JSON
	jsonData, err := json.Marshal(clientStats)
	if err != nil {
		return models.RegisterResponse{}, fmt.Errorf("error marshaling client stats: %w", err)
	}

	redisConn := repo.services.Redis

	// Store in Redis hash and set TTL
	err = redisConn.HSet(ctx, ClientsKey, clientID, jsonData).Err()
	if err != nil {
		return models.RegisterResponse{}, fmt.Errorf("error storing client: %w", err)
	}

	// Set TTL on the hash
	err = redisConn.Expire(ctx, ClientsKey, ClientsTTL).Err()
	if err != nil {
		return models.RegisterResponse{}, fmt.Errorf("error setting TTL: %w", err)
	}

	return models.RegisterResponse{ClientID: clientID}, nil
}

// UpdateHeartbeat updates the last_heartbeat timestamp for a client
func (repo *ClientRepository) UpdateHeartbeat(ctx context.Context, clientID string) error {

	redisConn := repo.services.Redis

	// Get current client stats
	jsonData, err := redisConn.HGet(ctx, ClientsKey, clientID).Bytes()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("client %s not found", clientID)
		}
		return fmt.Errorf("error getting client: %w", err)
	}

	var clientStats models.ClientStats
	err = json.Unmarshal(jsonData, &clientStats)
	if err != nil {
		return fmt.Errorf("error unmarshaling client stats: %w", err)
	}

	// Update last active time
	clientStats.LastActive = time.Now()

	// Convert back to JSON
	jsonData, err = json.Marshal(clientStats)
	if err != nil {
		return fmt.Errorf("error marshaling client stats: %w", err)
	}

	// Update in Redis and reset TTL
	err = redisConn.HSet(ctx, ClientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = redisConn.Expire(ctx, ClientsKey, ClientsTTL).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// GetClientStatsList retrieves statistics for all clients
func (repo *ClientRepository) GetClientStatsList(ctx context.Context) (models.StatsResponse, error) {

	redisConn := repo.services.Redis

	// Get all clients from Redis hash
	clients, err := redisConn.HGetAll(ctx, ClientsKey).Result()
	if err != nil {
		return models.StatsResponse{}, fmt.Errorf("error getting clients: %w", err)
	}

	stats := make([]models.ClientStats, 0, len(clients))
	for _, jsonData := range clients {
		var clientStats models.ClientStats
		err := json.Unmarshal([]byte(jsonData), &clientStats)
		if err != nil {
			return models.StatsResponse{}, fmt.Errorf("error unmarshaling client stats: %w", err)
		}
		stats = append(stats, clientStats)
	}

	// Sort such that the most recently active clients are first
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].LastActive.After(stats[j].LastActive)
	})

	return models.StatsResponse{
		ActiveClients: len(stats),
		ClientStats:   stats,
	}, nil
}

var ErrClientNotFound = errors.New("client not found")

// GetClientStats retrieves statistics for a specific client
func (repo *ClientRepository) GetClientStats(ctx context.Context, clientID string) (models.ClientStats, error) {
	redisConn := repo.services.Redis

	_, err := redisConn.HGet(ctx, ClientsKey, clientID).Bytes()

	if err == redis.Nil {
		return models.ClientStats{}, ErrClientNotFound
	}

	if err != nil {
		return models.ClientStats{}, fmt.Errorf("error getting client: %w", err)
	}

	return models.ClientStats{}, nil
}

// AssignJob assigns a job to a client
func (repo *ClientRepository) AssignJob(ctx context.Context, clientID string, job models.Job) error {
	redisConn := repo.services.Redis

	// Get current client stats
	jsonData, err := redisConn.HGet(ctx, ClientsKey, clientID).Bytes()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("client %s not found", clientID)
		}
		return fmt.Errorf("error getting client: %w", err)
	}

	var clientStats models.ClientStats
	err = json.Unmarshal(jsonData, &clientStats)
	if err != nil {
		return fmt.Errorf("error unmarshaling client stats: %w", err)
	}

	// Update position
	clientStats.Position = job.Position

	// Convert back to JSON
	jsonData, err = json.Marshal(clientStats)
	if err != nil {
		return fmt.Errorf("error marshaling client stats: %w", err)
	}

	// Update in Redis and reset TTL
	err = redisConn.HSet(ctx, ClientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = redisConn.Expire(ctx, ClientsKey, ClientsTTL).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// CompleteJob marks a job as completed and updates client stats
func (repo *ClientRepository) CompleteJob(ctx context.Context, clientID string) error {
	redisConn := repo.services.Redis
	// Get current client stats
	jsonData, err := redisConn.HGet(ctx, ClientsKey, clientID).Bytes()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("client %s not found", clientID)
		}
		return fmt.Errorf("error getting client: %w", err)
	}

	var clientStats models.ClientStats
	err = json.Unmarshal(jsonData, &clientStats)
	if err != nil {
		return fmt.Errorf("error unmarshaling client stats: %w", err)
	}

	// Update positions computed
	clientStats.PositionsComputed++

	// Convert back to JSON
	jsonData, err = json.Marshal(clientStats)
	if err != nil {
		return fmt.Errorf("error marshaling client stats: %w", err)
	}

	// Update in Redis and reset TTL
	err = redisConn.HSet(ctx, ClientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = redisConn.Expire(ctx, ClientsKey, ClientsTTL).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// GetTakenPositions returns all the positions that have been taken by clients
func (r *ClientRepository) GetTakenPositionsBytes(ctx context.Context) [][]byte {
	redisConn := r.services.Redis

	// Get all clients from Redis hash
	clients, err := redisConn.HGetAll(ctx, ClientsKey).Result()
	if err != nil {
		return nil
	}

	takenPositionsBytes := make([][]byte, 0, len(clients))
	for _, jsonData := range clients {
		var clientStats models.ClientStats
		err := json.Unmarshal([]byte(jsonData), &clientStats)
		if err != nil {
			continue
		}
		takenPositionsBytes = append(takenPositionsBytes, clientStats.Position.Bytes())
	}

	return takenPositionsBytes
}
