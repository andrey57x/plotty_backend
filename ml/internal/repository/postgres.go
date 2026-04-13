package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MLRepository interface {
	CreateTask(ctx context.Context, id uuid.UUID, taskType, payload string) error
	UpdateTaskResult(ctx context.Context, taskID uuid.UUID, status string, result interface{}) error
	GetLorebook(ctx context.Context, storyID uuid.UUID) (string, error)
	UpsertLorebook(ctx context.Context, storyID, chapterID uuid.UUID, entities string) error
	UpdateSummary(ctx context.Context, storyID uuid.UUID, summary string) error

	UpsertChapterLorebook(ctx context.Context, chapterID, storyID uuid.UUID, contentHash, entities string) error
	GetChapterLoreHash(ctx context.Context, chapterID uuid.UUID) (string, error)
	GetMergedLore(ctx context.Context, chapterIDs []uuid.UUID) (string, error)
	DeleteStoryLore(ctx context.Context, storyID uuid.UUID) error
}

type postgresRepo struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) MLRepository {
	return &postgresRepo{db: db}
}

func (r *postgresRepo) CreateTask(ctx context.Context, id uuid.UUID, taskType, payload string) error {
	query := `
		INSERT INTO ai_tasks (id, task_type, payload, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	now := time.Now()
	_, err := r.db.Exec(ctx, query, id, taskType, payload, "processing", now, now)
	return err
}

func (r *postgresRepo) UpdateTaskResult(ctx context.Context, taskID uuid.UUID, status string, result interface{}) error {
	var jsonData []byte
	var err error
	if result != nil {
		jsonData, err = json.Marshal(result)
		if err != nil {
			return err
		}
	}
	_, err = r.db.Exec(ctx, `UPDATE ai_tasks SET status = $1, result = $2, updated_at = $3 WHERE id = $4`, status, jsonData, time.Now(), taskID)
	return err
}

func (r *postgresRepo) UpdateSummary(ctx context.Context, storyID uuid.UUID, summary string) error {
	query := `
		INSERT INTO story_lorebooks (story_id, summary, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (story_id) DO UPDATE 
		SET summary = $2, updated_at = $3
	`
	_, err := r.db.Exec(ctx, query, storyID, summary, time.Now())
	return err
}

func (r *postgresRepo) GetLorebook(ctx context.Context, storyID uuid.UUID) (string, error) {
	return "{}", nil
}

func (r *postgresRepo) UpsertLorebook(ctx context.Context, storyID, chapterID uuid.UUID, entities string) error {
	return nil
}

func (r *postgresRepo) UpsertChapterLorebook(ctx context.Context, chapterID, storyID uuid.UUID, contentHash, entities string) error {
	query := `
		INSERT INTO chapter_lorebooks (chapter_id, story_id, content_hash, entities, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (chapter_id) DO UPDATE 
		SET entities = $4, content_hash = $3, updated_at = $5
	`
	_, err := r.db.Exec(ctx, query, chapterID, storyID, contentHash, entities, time.Now())
	return err
}

func (r *postgresRepo) GetChapterLoreHash(ctx context.Context, chapterID uuid.UUID) (string, error) {
	var hash string
	err := r.db.QueryRow(ctx, "SELECT content_hash FROM chapter_lorebooks WHERE chapter_id = $1", chapterID).Scan(&hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return hash, nil
}

func (r *postgresRepo) GetMergedLore(ctx context.Context, chapterIDs []uuid.UUID) (string, error) {
	if len(chapterIDs) == 0 {
		return "{}", nil
	}

	rows, err := r.db.Query(ctx, "SELECT entities FROM chapter_lorebooks WHERE chapter_id = ANY($1) ORDER BY updated_at ASC", chapterIDs)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	type Entity struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	type Lore struct {
		Characters []Entity `json:"characters"`
		Locations  []Entity `json:"locations"`
		Items      []Entity `json:"items"`
	}

	charMap := make(map[string]string)
	locMap := make(map[string]string)
	itemMap := make(map[string]string)

	for rows.Next() {
		var rawJSON string
		if err := rows.Scan(&rawJSON); err != nil {
			continue
		}
		var l Lore
		if err := json.Unmarshal([]byte(rawJSON), &l); err == nil {
			for _, c := range l.Characters {
				charMap[c.Name] = c.State
			}
			for _, c := range l.Locations {
				locMap[c.Name] = c.State
			}
			for _, c := range l.Items {
				itemMap[c.Name] = c.State
			}
		}
	}

	merged := Lore{}
	for k, v := range charMap {
		merged.Characters = append(merged.Characters, Entity{Name: k, State: v})
	}
	for k, v := range locMap {
		merged.Locations = append(merged.Locations, Entity{Name: k, State: v})
	}
	for k, v := range itemMap {
		merged.Items = append(merged.Items, Entity{Name: k, State: v})
	}

	out, _ := json.Marshal(merged)
	return string(out), nil
}

func (r *postgresRepo) DeleteStoryLore(ctx context.Context, storyID uuid.UUID) error {
	_, err := r.db.Exec(ctx, "DELETE FROM chapter_lorebooks WHERE story_id = $1", storyID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, "DELETE FROM story_lorebooks WHERE story_id = $1", storyID)
	return err
}
