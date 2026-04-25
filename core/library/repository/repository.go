package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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

type ShelfRow struct {
	StoryID   uuid.UUID
	Shelf     string
	UpdatedAt time.Time
}

func (r *Repository) UpsertShelf(ctx context.Context, userID uint64, storyID uuid.UUID, shelf string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO reader_story_shelf (user_id, story_id, shelf, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, story_id) DO UPDATE
		SET shelf = EXCLUDED.shelf, updated_at = CURRENT_TIMESTAMP
	`, userID, storyID, shelf)
	if err != nil {
		return fmt.Errorf("library_repo.UpsertShelf: %w", err)
	}
	return nil
}

func (r *Repository) RemoveShelf(ctx context.Context, userID uint64, storyID uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `
		DELETE FROM reader_story_shelf WHERE user_id = $1 AND story_id = $2
	`, userID, storyID)
	if err != nil {
		return fmt.Errorf("library_repo.RemoveShelf: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	return nil
}

func (r *Repository) ListShelfRows(ctx context.Context, userID uint64, shelfFilter *string) ([]ShelfRow, error) {
	q := `
		SELECT story_id, shelf, updated_at
		FROM reader_story_shelf
		WHERE user_id = $1
	`
	args := []any{userID}
	if shelfFilter != nil && strings.TrimSpace(*shelfFilter) != "" {
		q += ` AND shelf = $2`
		args = append(args, strings.TrimSpace(*shelfFilter))
	}
	q += ` ORDER BY updated_at DESC`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("library_repo.ListShelfRows: %w", err)
	}
	defer rows.Close()

	var out []ShelfRow
	for rows.Next() {
		var row ShelfRow
		if err := rows.Scan(&row.StoryID, &row.Shelf, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) InsertCollection(ctx context.Context, c models.UserCollection) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_collections (id, user_id, title, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, c.ID, c.UserID, c.Title, c.Description, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("library_repo.InsertCollection: %w", err)
	}
	return nil
}

func (r *Repository) UpdateCollection(ctx context.Context, userID uint64, id uuid.UUID, title *string, description *string) (*models.UserCollection, error) {
	cur, err := r.GetCollectionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.UserID != userID {
		return nil, named_errors.ErrNoAccess
	}
	newTitle := cur.Title
	if title != nil {
		newTitle = strings.TrimSpace(*title)
		if newTitle == "" {
			return nil, named_errors.ErrInvalidInput
		}
	}
	newDesc := cur.Description
	if description != nil {
		d := strings.TrimSpace(*description)
		if d == "" {
			newDesc = nil
		} else {
			newDesc = &d
		}
	}
	now := time.Now().UTC()
	_, err = r.pool.Exec(ctx, `
		UPDATE user_collections
		SET title = $2, description = $3, updated_at = $4
		WHERE id = $1
	`, id, newTitle, newDesc, now)
	if err != nil {
		return nil, fmt.Errorf("library_repo.UpdateCollection: %w", err)
	}
	return r.GetCollectionByID(ctx, id)
}

func (r *Repository) DeleteCollection(ctx context.Context, userID uint64, id uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `
		DELETE FROM user_collections WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return fmt.Errorf("library_repo.DeleteCollection: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	return nil
}

func (r *Repository) GetCollectionByID(ctx context.Context, id uuid.UUID) (*models.UserCollection, error) {
	var c models.UserCollection
	var desc sql.NullString
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, title, description, created_at, updated_at
		FROM user_collections WHERE id = $1
	`, id).Scan(&c.ID, &c.UserID, &c.Title, &desc, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("library_repo.GetCollectionByID: %w", err)
	}
	if desc.Valid {
		c.Description = &desc.String
	}
	return &c, nil
}

func (r *Repository) ListCollectionSummaries(ctx context.Context, userID uint64) ([]models.UserCollectionSummary, error) {
	q := `
		SELECT c.id, c.user_id, c.title, c.description, c.created_at, c.updated_at,
			(SELECT COUNT(*)::int FROM user_collection_stories s WHERE s.collection_id = c.id) AS stories_count
		FROM user_collections c
		WHERE c.user_id = $1
		ORDER BY c.updated_at DESC
	`

	rows, err := r.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("library_repo.ListCollectionSummaries: %w", err)
	}
	defer rows.Close()

	var out []models.UserCollectionSummary
	for rows.Next() {
		var s models.UserCollectionSummary
		var desc sql.NullString
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &desc, &s.CreatedAt, &s.UpdatedAt, &s.StoriesCount); err != nil {
			return nil, err
		}
		if desc.Valid {
			s.Description = &desc.String
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) ListCollectionStoryIDs(ctx context.Context, collectionID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ucs.story_id
		FROM user_collection_stories ucs
		JOIN stories s ON s.id = ucs.story_id
		WHERE ucs.collection_id = $1
		ORDER BY s.updated_at DESC, ucs.story_id ASC
	`, collectionID)
	if err != nil {
		return nil, fmt.Errorf("library_repo.ListCollectionStoryIDs: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) AddCollectionStory(ctx context.Context, collectionID, storyID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_collection_stories (collection_id, story_id, created_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (collection_id, story_id) DO NOTHING
	`, collectionID, storyID)
	if err != nil {
		return fmt.Errorf("library_repo.AddCollectionStory: %w", err)
	}
	return nil
}

func (r *Repository) RemoveCollectionStory(ctx context.Context, collectionID, storyID uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `
		DELETE FROM user_collection_stories WHERE collection_id = $1 AND story_id = $2
	`, collectionID, storyID)
	if err != nil {
		return fmt.Errorf("library_repo.RemoveCollectionStory: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	return nil
}
