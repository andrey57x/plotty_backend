package repository

import (
	"context"
	"fmt"

	"github.com/fivecode/plotty/core/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Like(ctx context.Context, userID uint64, storyID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO story_likes (user_id, story_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, userID, storyID)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Uint64("user_id", userID).Stringer("story_id", storyID).Msg("like_repo: insert failed")
		return fmt.Errorf("like_repo.Like: %w", err)
	}
	return nil
}

func (r *Repository) Unlike(ctx context.Context, userID uint64, storyID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM story_likes WHERE user_id = $1 AND story_id = $2
	`, userID, storyID)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Uint64("user_id", userID).Stringer("story_id", storyID).Msg("like_repo: delete failed")
		return fmt.Errorf("like_repo.Unlike: %w", err)
	}
	return nil
}

func (r *Repository) Count(ctx context.Context, storyID uuid.UUID) (int, error) {
	var c int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM story_likes WHERE story_id = $1`, storyID).Scan(&c)
	if err != nil {
		return 0, fmt.Errorf("like_repo.Count: %w", err)
	}
	return c, nil
}

func (r *Repository) IsLiked(ctx context.Context, userID uint64, storyID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM story_likes WHERE user_id = $1 AND story_id = $2)
	`, userID, storyID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("like_repo.IsLiked: %w", err)
	}
	return exists, nil
}
