package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MLRepository interface {
	CreateTask(ctx context.Context, id uuid.UUID, taskType, payload string) error
	UpdateTaskResult(ctx context.Context, taskID uuid.UUID, status string, result interface{}) error
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
		return fmt.Errorf("ошибка выполнения обновления задачи: %w", err)
	}
	return nil
}
