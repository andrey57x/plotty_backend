package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, chapterID uuid.UUID, userID uint64, content string) (*models.Comment, error) {
	log := logger.FromContext(ctx)

	id := uuid.New()
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO chapter_comments (id, chapter_id, user_id, content, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, id, chapterID, userID, content, now)
	if err != nil {
		log.Error().Err(err).Stringer("chapter_id", chapterID).Uint64("user_id", userID).Msg("comment_repo: insert failed")
		return nil, fmt.Errorf("comment_repo.Create insert: %w", err)
	}

	var c models.Comment
	err = r.pool.QueryRow(ctx, `
		SELECT cc.id, cc.chapter_id, cc.user_id, u.username, u.avatar_url, cc.content, cc.created_at, cc.updated_at
		FROM chapter_comments cc
		JOIN users u ON u.id = cc.user_id
		WHERE cc.id = $1
	`, id).Scan(&c.ID, &c.ChapterID, &c.UserID, &c.Username, &c.AvatarURL, &c.Content, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		log.Error().Err(err).Stringer("comment_id", id).Msg("comment_repo: select after insert failed")
		return nil, fmt.Errorf("comment_repo.Create select: %w", err)
	}

	log.Info().Stringer("comment_id", id).Stringer("chapter_id", chapterID).Msg("comment_repo: created")
	return &c, nil
}

func (r *Repository) ListByChapter(ctx context.Context, chapterID uuid.UUID, limit, offset int) ([]models.Comment, int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM chapter_comments WHERE chapter_id = $1
	`, chapterID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("comment_repo.ListByChapter count: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT cc.id, cc.chapter_id, cc.user_id, u.username, u.avatar_url, cc.content, cc.created_at, cc.updated_at
		FROM chapter_comments cc
		JOIN users u ON u.id = cc.user_id
		WHERE cc.chapter_id = $1
		ORDER BY cc.created_at ASC
		LIMIT $2 OFFSET $3
	`, chapterID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("comment_repo.ListByChapter query: %w", err)
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.ChapterID, &c.UserID, &c.Username, &c.AvatarURL, &c.Content, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("comment_repo.ListByChapter scan: %w", err)
		}
		comments = append(comments, c)
	}
	if comments == nil {
		comments = []models.Comment{}
	}
	return comments, total, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, commentID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM chapter_comments WHERE id = $1`, commentID)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("comment_id", commentID).Msg("comment_repo: delete failed")
		return fmt.Errorf("comment_repo.Delete: %w", err)
	}
	logger.Ctx(ctx).Info().Stringer("comment_id", commentID).Msg("comment_repo: deleted")
	return nil
}

func (r *Repository) Update(ctx context.Context, commentID uuid.UUID, content string) (*models.Comment, error) {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE chapter_comments SET content = $1, updated_at = $2 WHERE id = $3
	`, content, now, commentID)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("comment_id", commentID).Msg("comment_repo: update failed")
		return nil, fmt.Errorf("comment_repo.Update exec: %w", err)
	}

	var c models.Comment
	err = r.pool.QueryRow(ctx, `
		SELECT cc.id, cc.chapter_id, cc.user_id, u.username, u.avatar_url, cc.content, cc.created_at, cc.updated_at
		FROM chapter_comments cc
		JOIN users u ON u.id = cc.user_id
		WHERE cc.id = $1
	`, commentID).Scan(&c.ID, &c.ChapterID, &c.UserID, &c.Username, &c.AvatarURL, &c.Content, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("comment_id", commentID).Msg("comment_repo: select after update failed")
		return nil, fmt.Errorf("comment_repo.Update select: %w", err)
	}
	return &c, nil
}

func (r *Repository) GetOwnerID(ctx context.Context, commentID uuid.UUID) (uint64, error) {
	var userID uint64
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM chapter_comments WHERE id = $1`, commentID).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("comment_repo.GetOwnerID: %w", err)
	}
	return userID, nil
}
