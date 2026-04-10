package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if err != nil {
		return fmt.Errorf("ошибка создания записи задачи: %w", err)
	}
	return nil
}

func (r *postgresRepo) UpdateTaskResult(ctx context.Context, taskID uuid.UUID, status string, result interface{}) error {
	var jsonData []byte
	var err error

	if result != nil {
		jsonData, err = json.Marshal(result)
		if err != nil {
			return fmt.Errorf("ошибка сериализации результата: %w", err)
		}
	}

	query := `
		UPDATE ai_tasks 
		SET status = $1, result = $2, updated_at = $3 
		WHERE id = $4
	`
	_, err = r.db.Exec(ctx, query, status, jsonData, time.Now(), taskID)
	if err != nil {
		return fmt.Errorf("ошибка обновления задачи: %w", err)
	}
	return nil
}

func (r *postgresRepo) GetLorebook(ctx context.Context, storyID uuid.UUID) (string, error) {
	var entities string
	err := r.db.QueryRow(ctx, "SELECT entities::text FROM story_lorebooks WHERE story_id = $1", storyID).Scan(&entities)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "{}", nil
		}
		return "", err
	}
	return entities, nil
}

func (r *postgresRepo) UpsertLorebook(ctx context.Context, storyID, chapterID uuid.UUID, entities string) error {
	query := `
		INSERT INTO story_lorebooks (story_id, entities, last_processed_chapter_id, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (story_id) DO UPDATE 
		SET entities = $2, last_processed_chapter_id = $3, updated_at = $4
	`
	_, err := r.db.Exec(ctx, query, storyID, entities, chapterID, time.Now())
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
