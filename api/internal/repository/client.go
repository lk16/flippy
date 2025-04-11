package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lk16/flippy/api/internal/models"
	"github.com/redis/go-redis/v9"
)

const (
	clientsKey = "clients"
	ttl        = 300 * time.Second
)

type ClientRepository struct {
	redis *redis.Client
}

func NewClientRepository(redis *redis.Client) *ClientRepository {
	return &ClientRepository{
		redis: redis,
	}
}

// RegisterClient registers a new client and returns its ID
func (r *ClientRepository) RegisterClient(ctx context.Context, req models.RegisterRequest) (models.RegisterResponse, error) {
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

	// Store in Redis hash and set TTL
	err = r.redis.HSet(ctx, clientsKey, clientID, jsonData).Err()
	if err != nil {
		return models.RegisterResponse{}, fmt.Errorf("error storing client: %w", err)
	}

	// Set TTL on the hash
	err = r.redis.Expire(ctx, clientsKey, ttl).Err()
	if err != nil {
		return models.RegisterResponse{}, fmt.Errorf("error setting TTL: %w", err)
	}

	return models.RegisterResponse{ClientID: clientID}, nil
}

// UpdateHeartbeat updates the last_heartbeat timestamp for a client
func (r *ClientRepository) UpdateHeartbeat(ctx context.Context, clientID string) error {
	// Get current client stats
	jsonData, err := r.redis.HGet(ctx, clientsKey, clientID).Bytes()
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
	err = r.redis.HSet(ctx, clientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = r.redis.Expire(ctx, clientsKey, ttl).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// GetClientStats retrieves statistics for all clients
func (r *ClientRepository) GetClientStats(ctx context.Context) (models.StatsResponse, error) {
	// Get all clients from Redis hash
	clients, err := r.redis.HGetAll(ctx, clientsKey).Result()
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

// AssignJob assigns a job to a client
func (r *ClientRepository) AssignJob(ctx context.Context, clientID string, job models.Job) error {
	// Get current client stats
	jsonData, err := r.redis.HGet(ctx, clientsKey, clientID).Bytes()
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
	err = r.redis.HSet(ctx, clientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = r.redis.Expire(ctx, clientsKey, ttl).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// CompleteJob marks a job as completed and updates client stats
func (r *ClientRepository) CompleteJob(ctx context.Context, clientID string) error {
	// Get current client stats
	jsonData, err := r.redis.HGet(ctx, clientsKey, clientID).Bytes()
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
	err = r.redis.HSet(ctx, clientsKey, clientID, jsonData).Err()
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}

	err = r.redis.Expire(ctx, clientsKey, ttl).Err()
	if err != nil {
		return fmt.Errorf("error setting TTL: %w", err)
	}

	return nil
}

// GetTakenPositions returns all the positions that have been taken by clients
func (r *ClientRepository) GetTakenPositionsBytes(ctx context.Context) [][]byte {
	// Get all clients from Redis hash
	clients, err := r.redis.HGetAll(ctx, clientsKey).Result()
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
