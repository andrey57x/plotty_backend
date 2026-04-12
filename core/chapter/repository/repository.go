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

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Chapter, error) {
	var c models.Chapter
	err := r.pool.QueryRow(ctx, `
		SELECT id, story_id, title, content, status, created_at, updated_at
		FROM chapters WHERE id = $1
	`, id).Scan(
		&c.ID, &c.StoryID, &c.Title, &c.Content, &c.Status, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("chapter_id", id).Msg("chapter_repo: GetByID failed")
		return nil, fmt.Errorf("chapter_repo.GetByID: %w", err)
	}
	return &c, nil
}

func (r *Repository) Create(ctx context.Context, storyID uuid.UUID, title, content string) (*models.Chapter, error) {
	log := logger.FromContext(ctx)
	id := uuid.New()
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO chapters (id, story_id, title, content, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, storyID, title, content, "draft", now, now)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", storyID).Msg("chapter_repo: insert failed")
		return nil, fmt.Errorf("chapter_repo.Create: %w", err)
	}

	log.Info().Stringer("chapter_id", id).Stringer("story_id", storyID).Msg("chapter_repo: created")
	return &models.Chapter{
		ID: id, StoryID: storyID, Title: title, Content: content, Status: "draft",
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, title *string, content *string) (*models.Chapter, error) {
	log := logger.FromContext(ctx)

	c, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("chapter_repo.Update get current: %w", err)
	}
	newTitle := c.Title
	newContent := c.Content
	if title != nil {
		newTitle = *title
	}
	if content != nil {
		newContent = *content
	}
	now := time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, `
		UPDATE chapters SET title = $2, content = $3, updated_at = $4
		WHERE id = $1
	`, id, newTitle, newContent, now)
	if err != nil {
		log.Error().Err(err).Stringer("chapter_id", id).Msg("chapter_repo: update failed")
		return nil, fmt.Errorf("chapter_repo.Update: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return nil, named_errors.ErrNotFound
	}
	c.Title = newTitle
	c.Content = newContent
	c.UpdatedAt = now

	log.Info().Stringer("chapter_id", id).Msg("chapter_repo: updated")
	return c, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM chapters WHERE id = $1`, id)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("chapter_id", id).Msg("chapter_repo: delete failed")
		return fmt.Errorf("chapter_repo.Delete: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	logger.Ctx(ctx).Info().Stringer("chapter_id", id).Msg("chapter_repo: deleted")
	return nil
}

func (r *Repository) GetLatestImageURL(ctx context.Context, chapterID uuid.UUID) (*string, error) {
	var url string
	err := r.pool.QueryRow(ctx, `
		SELECT image_url FROM generated_images
		WHERE chapter_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, chapterID).Scan(&url)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chapter_repo.GetLatestImageURL: %w", err)
	}
	return &url, nil
}

func (r *Repository) ListBriefByStory(ctx context.Context, storyID uuid.UUID) ([]models.ChapterBrief, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, status, updated_at
		FROM chapters WHERE story_id = $1
		ORDER BY created_at ASC, id ASC
	`, storyID)
	if err != nil {
		return nil, fmt.Errorf("chapter_repo.ListBriefByStory: %w", err)
	}
	defer rows.Close()
	var out []models.ChapterBrief
	for rows.Next() {
		var b models.ChapterBrief
		if err := rows.Scan(&b.ID, &b.Title, &b.Status, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("chapter_repo.ListBriefByStory scan: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repository) Publish(ctx context.Context, id uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `UPDATE chapters SET status = 'published', updated_at = $2 WHERE id = $1`, id, time.Now().UTC())
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("chapter_id", id).Msg("chapter_repo: publish failed")
		return fmt.Errorf("chapter_repo.Publish: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	logger.Ctx(ctx).Info().Stringer("chapter_id", id).Msg("chapter_repo: published")
	return nil
}
