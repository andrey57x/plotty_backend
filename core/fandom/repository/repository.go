package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, f models.SuggestedFandom) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO suggested_fandoms (id, user_id, name, description, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, f.ID, f.UserID, f.Name, f.Description, f.Status, f.CreatedAt, f.UpdatedAt)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("fandom_repo: create failed")
		return fmt.Errorf("fandom_repo.Create: %w", err)
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.SuggestedFandom, error) {
	var f models.SuggestedFandom
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, name, description, status, created_at, updated_at
		FROM suggested_fandoms WHERE id = $1
	`, id).Scan(&f.ID, &f.UserID, &f.Name, &f.Description, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fandom_repo.GetByID: %w", err)
	}
	return &f, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	cmd, err := r.pool.Exec(ctx, `
		UPDATE suggested_fandoms SET status = $2, updated_at = $3 WHERE id = $1
	`, id, status, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("fandom_repo.UpdateStatus: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	return nil
}

func (r *Repository) ListPending(ctx context.Context, limit, offset int) ([]models.SuggestedFandom, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, name, description, status, created_at, updated_at
		FROM suggested_fandoms
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("fandom_repo.ListPending: %w", err)
	}
	defer rows.Close()

	var out []models.SuggestedFandom
	for rows.Next() {
		var f models.SuggestedFandom
		if err := rows.Scan(&f.ID, &f.UserID, &f.Name, &f.Description, &f.Status, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
