package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lk16/flippy/api/internal/db"
	"github.com/lk16/flippy/api/internal/models"
)

type ClientRepository struct {
	db *sqlx.DB
}

func NewClientRepository() *ClientRepository {
	return &ClientRepository{
		db: db.GetDB(),
	}
}

// RegisterClient registers a new client and returns its ID
func (r *ClientRepository) RegisterClient(ctx context.Context, req models.RegisterRequest) (models.RegisterResponse, error) {
	clientID := uuid.New().String()

	query := `
		INSERT INTO clients (id, hostname, git_commit, position, last_heartbeat)
		VALUES ($1, $2, $3, NULL, NOW())
	`

	_, err := r.db.ExecContext(ctx, query, clientID, req.Hostname, req.GitCommit)
	if err != nil {
		return models.RegisterResponse{}, fmt.Errorf("error registering client: %w", err)
	}

	return models.RegisterResponse{ClientID: clientID}, nil
}

// UpdateHeartbeat updates the last_heartbeat timestamp for a client
func (r *ClientRepository) UpdateHeartbeat(ctx context.Context, clientID string) error {
	query := `
		UPDATE clients
		SET last_heartbeat = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, clientID)
	if err != nil {
		return fmt.Errorf("error updating heartbeat: %w", err)
	}

	return nil
}

// GetClientStats retrieves statistics for all clients
func (r *ClientRepository) GetClientStats(ctx context.Context) (models.StatsResponse, error) {
	query := `
		SELECT
			id,
			hostname,
			git_commit,
			jobs_completed as positions_computed,
			last_heartbeat as last_active
		FROM clients
		ORDER BY jobs_completed DESC
	`

	var stats []models.ClientStats
	err := r.db.SelectContext(ctx, &stats, query)
	if err != nil {
		return models.StatsResponse{}, fmt.Errorf("error getting client stats: %w", err)
	}

	return models.StatsResponse{
		ActiveClients: len(stats),
		ClientStats:   stats,
	}, nil
}

// PruneInactiveClients removes clients that haven't sent a heartbeat in 5 minutes
func (r *ClientRepository) PruneInactiveClients(ctx context.Context) error {
	query := `
		DELETE FROM clients
		WHERE last_heartbeat < CURRENT_TIMESTAMP - INTERVAL '5 minutes'
		OR last_heartbeat IS NULL
	`

	_, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("error pruning inactive clients: %w", err)
	}

	return nil
}

// AssignJob assigns a job to a client
func (r *ClientRepository) AssignJob(ctx context.Context, clientID string, job models.Job) error {
	query := `
		UPDATE clients
		SET position = $1
		WHERE id = $2
	`

	_, err := r.db.ExecContext(ctx, query, job.Position, clientID)
	if err != nil {
		return fmt.Errorf("error assigning job: %w", err)
	}

	return nil
}

// CompleteJob marks a job as completed and updates client stats
func (r *ClientRepository) CompleteJob(ctx context.Context, clientID string) error {
	query := `
		UPDATE clients
		SET jobs_completed = jobs_completed + 1,
			position = NULL
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, clientID)
	if err != nil {
		return fmt.Errorf("error completing job: %w", err)
	}

	return nil
}
