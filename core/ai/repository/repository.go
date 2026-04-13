package repository

import (
	"context"
	"encoding/json"
	"errors"
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

func (r *Repository) CreateJob(ctx context.Context, job models.AIJob) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ai_jobs (id, type, status, story_id, chapter_id, input_payload, result_payload, error_message, content_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, job.ID, job.Type, job.Status, job.StoryID, job.ChapterID, job.InputPayload, job.ResultPayload, job.ErrorMessage, job.ContentHash, job.CreatedAt, job.UpdatedAt)
	return err
}

func (r *Repository) GetCompletedJobByHash(ctx context.Context, chapterID uuid.UUID, jobType, hash string) (*models.AIJob, error) {
	var j models.AIJob
	err := r.pool.QueryRow(ctx, `
		SELECT id, type, status, story_id, chapter_id, input_payload, result_payload, error_message, content_hash, created_at, updated_at
		FROM ai_jobs 
		WHERE chapter_id = $1 AND type = $2 AND content_hash = $3 AND status = 'completed'
		ORDER BY created_at DESC LIMIT 1
	`, chapterID, jobType, hash).Scan(
		&j.ID, &j.Type, &j.Status, &j.StoryID, &j.ChapterID, &j.InputPayload, &j.ResultPayload, &j.ErrorMessage, &j.ContentHash, &j.CreatedAt, &j.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *Repository) UpdateJob(ctx context.Context, id uuid.UUID, status string, result []byte, errMsg *string) error {
	now := time.Now().UTC()
	cmd, err := r.pool.Exec(ctx, `
		UPDATE ai_jobs SET status = $2, result_payload = $3, error_message = $4, updated_at = $5
		WHERE id = $1
	`, id, status, result, errMsg, now)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	return nil
}

func (r *Repository) GetJob(ctx context.Context, id uuid.UUID) (*models.AIJob, error) {
	var j models.AIJob
	err := r.pool.QueryRow(ctx, `
		SELECT id, type, status, story_id, chapter_id, input_payload, result_payload, error_message, content_hash, created_at, updated_at
		FROM ai_jobs WHERE id = $1
	`, id).Scan(
		&j.ID, &j.Type, &j.Status, &j.StoryID, &j.ChapterID, &j.InputPayload, &j.ResultPayload, &j.ErrorMessage, &j.ContentHash, &j.CreatedAt, &j.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *Repository) InsertGeneratedImage(ctx context.Context, img models.GeneratedImage) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO generated_images (id, job_id, chapter_id, prompt, image_url, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, img.ID, img.JobID, img.ChapterID, img.Prompt, img.ImageURL, img.CreatedAt)
	return err
}

func (r *Repository) ListImagesByJob(ctx context.Context, jobID uuid.UUID) ([]models.GeneratedImage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, job_id, chapter_id, prompt, image_url, created_at
		FROM generated_images WHERE job_id = $1 ORDER BY created_at ASC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.GeneratedImage
	for rows.Next() {
		var g models.GeneratedImage
		if err := rows.Scan(&g.ID, &g.JobID, &g.ChapterID, &g.Prompt, &g.ImageURL, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func MergeResultPayloadImages(base []byte, images []models.GeneratedImage) ([]byte, error) {
	var m map[string]any
	if len(base) > 0 {
		if err := json.Unmarshal(base, &m); err != nil {
			m = map[string]any{}
		}
	} else {
		m = map[string]any{}
	}
	type imgDTO struct {
		ID       string `json:"id"`
		ImageURL string `json:"imageUrl"`
		Prompt   string `json:"prompt"`
	}
	list := make([]imgDTO, 0, len(images))
	for _, im := range images {
		list = append(list, imgDTO{ID: im.ID.String(), ImageURL: im.ImageURL, Prompt: im.Prompt})
	}
	m["images"] = list
	return json.Marshal(m)
}
