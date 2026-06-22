package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fivecode/plotty/ml/internal/models"
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

	GetLoreByChapterID(ctx context.Context, chapterID uuid.UUID) (string, error)
	SearchCanonFacts(ctx context.Context, fandomSlug string, embedding []float32, limit int) ([]string, error)

	GetStoryLoreNames(ctx context.Context, storyID uuid.UUID) ([]string, error)
	GetCanonEntityNames(ctx context.Context, fandomSlug string) ([]string, error)

	UpdateSummaryAndEmbedding(ctx context.Context, storyID uuid.UUID, summary string, embedding []float32) error
	GetSimilarStories(ctx context.Context, storyID uuid.UUID, limit int) ([]uuid.UUID, error)

	SearchStoriesByEmbedding(ctx context.Context, embedding []float32, limit int) ([]uuid.UUID, error)
	InsertCanonFact(ctx context.Context, fact models.CanonFact) error

	// Новые сигнатуры методов без эмбеддингов
	GetGlobalCanonFacts(ctx context.Context, fandomSlug string) ([]string, error)
	GetCanonFactsByEntities(ctx context.Context, fandomSlug string, entities []string) ([]string, error)
}

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) MLRepository {
	return &postgresRepo{pool: pool}
}

func (r *postgresRepo) CreateTask(ctx context.Context, id uuid.UUID, taskType, payload string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ai_tasks (id, task_type, payload, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
	`, id, taskType, payload, "processing", now)
	return err
}

func (r *postgresRepo) UpdateTaskResult(ctx context.Context, taskID uuid.UUID, status string, result interface{}) error {
	now := time.Now().UTC()
	resJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE ai_tasks SET status = $2, result = $3, updated_at = $4 WHERE id = $1
	`, taskID, status, resJSON, now)
	return err
}

func (r *postgresRepo) GetLorebook(ctx context.Context, storyID uuid.UUID) (string, error) {
	var out string
	err := r.pool.QueryRow(ctx, `
		SELECT entities::text FROM story_lorebooks WHERE story_id = $1
	`, storyID).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return "{}", nil
	}
	return out, err
}

func (r *postgresRepo) UpsertLorebook(ctx context.Context, storyID, chapterID uuid.UUID, entities string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO story_lorebooks (story_id, entities, last_processed_chapter_id, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (story_id) DO UPDATE SET
			entities = EXCLUDED.entities,
			last_processed_chapter_id = EXCLUDED.last_processed_chapter_id,
			updated_at = EXCLUDED.updated_at
	`, storyID, entities, chapterID, now)
	return err
}

func (r *postgresRepo) UpdateSummary(ctx context.Context, storyID uuid.UUID, summary string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO story_lorebooks (story_id, summary, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (story_id) DO UPDATE SET
			summary = EXCLUDED.summary,
			updated_at = EXCLUDED.updated_at
	`, storyID, summary, now)
	return err
}

func (r *postgresRepo) UpsertChapterLorebook(ctx context.Context, chapterID, storyID uuid.UUID, contentHash, entities string) error {
	query := `
		INSERT INTO chapter_lorebooks (chapter_id, story_id, content_hash, entities, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (chapter_id) DO UPDATE 
		SET entities = $4, content_hash = $3, updated_at = $5
	`
	_, err := r.pool.Exec(ctx, query, chapterID, storyID, contentHash, entities, time.Now().UTC())
	return err
}

func (r *postgresRepo) GetChapterLoreHash(ctx context.Context, chapterID uuid.UUID) (string, error) {
	var hash string
	err := r.pool.QueryRow(ctx, "SELECT content_hash FROM chapter_lorebooks WHERE chapter_id = $1", chapterID).Scan(&hash)
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

	rows, err := r.pool.Query(ctx, "SELECT entities FROM chapter_lorebooks WHERE chapter_id = ANY($1) ORDER BY updated_at ASC", chapterIDs)
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
	_, err := r.pool.Exec(ctx, "DELETE FROM chapter_lorebooks WHERE story_id = $1", storyID)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, "DELETE FROM story_lorebooks WHERE story_id = $1", storyID)
	return err
}

func (r *postgresRepo) GetLoreByChapterID(ctx context.Context, chapterID uuid.UUID) (string, error) {
	var entities string
	err := r.pool.QueryRow(ctx, "SELECT entities FROM chapter_lorebooks WHERE chapter_id = $1", chapterID).Scan(&entities)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "{}", nil
		}
		return "", err
	}
	return entities, nil
}

func vecToString(v []float32) string {
	s := make([]string, len(v))
	for i, val := range v {
		s[i] = fmt.Sprintf("%f", val)
	}
	return "[" + strings.Join(s, ",") + "]"
}

func (r *postgresRepo) SearchCanonFacts(ctx context.Context, fandomSlug string, embedding []float32, limit int) ([]string, error) {
	vecStr := vecToString(embedding)

	query := `
		SELECT fact_text 
		FROM canon_lorebooks 
		WHERE fandom_slug = $1 
		ORDER BY embedding <=> $2::vector 
		LIMIT $3
	`
	rows, err := r.pool.Query(ctx, query, fandomSlug, vecStr, limit)
	if err != nil {
		return nil, fmt.Errorf("db.Query error: %w", err)
	}
	defer rows.Close()

	var facts []string
	for rows.Next() {
		var fact string
		if err := rows.Scan(&fact); err != nil {
			return nil, fmt.Errorf("rows.Scan error: %w", err)
		}
		facts = append(facts, fact)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}

	return facts, nil
}

func (r *postgresRepo) GetStoryLoreNames(ctx context.Context, storyID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, "SELECT entities FROM chapter_lorebooks WHERE story_id = $1", storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	namesMap := make(map[string]struct{})
	type Entity struct {
		Name string `json:"name"`
	}
	type Lore struct {
		Characters []Entity `json:"characters"`
		Locations  []Entity `json:"locations"`
		Items      []Entity `json:"items"`
	}

	for rows.Next() {
		var rawJSON string
		if err := rows.Scan(&rawJSON); err != nil {
			continue
		}
		var l Lore
		if err := json.Unmarshal([]byte(rawJSON), &l); err == nil {
			for _, c := range l.Characters {
				namesMap[strings.ToLower(c.Name)] = struct{}{}
			}
			for _, c := range l.Locations {
				namesMap[strings.ToLower(c.Name)] = struct{}{}
			}
			for _, c := range l.Items {
				namesMap[strings.ToLower(c.Name)] = struct{}{}
			}
		}
	}

	var names []string
	for k := range namesMap {
		names = append(names, k)
	}
	return names, nil
}

func (r *postgresRepo) GetCanonEntityNames(ctx context.Context, fandomSlug string) ([]string, error) {
	if fandomSlug == "" {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, "SELECT entity_name FROM canon_lorebooks WHERE fandom_slug = $1 AND entity_name IS NOT NULL", fandomSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil && name != "" {
			names = append(names, strings.ToLower(name))
		}
	}
	return names, nil
}

func (r *postgresRepo) UpdateSummaryAndEmbedding(ctx context.Context, storyID uuid.UUID, summary string, embedding []float32) error {
	query := `
		INSERT INTO story_lorebooks (story_id, summary, embedding, updated_at)
		VALUES ($1, $2, $3::vector, $4)
		ON CONFLICT (story_id) DO UPDATE 
		SET summary = $2, embedding = $3::vector, updated_at = $4
	`
	vecStr := vecToString(embedding)
	_, err := r.pool.Exec(ctx, query, storyID, summary, vecStr, time.Now().UTC())
	return err
}

func (r *postgresRepo) GetSimilarStories(ctx context.Context, storyID uuid.UUID, limit int) ([]uuid.UUID, error) {
	var vecStr string
	err := r.pool.QueryRow(ctx, "SELECT embedding::text FROM story_lorebooks WHERE story_id = $1 AND embedding IS NOT NULL", storyID).Scan(&vecStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	query := `
		SELECT story_id 
		FROM story_lorebooks 
		WHERE story_id != $1 AND embedding IS NOT NULL
		ORDER BY embedding <=> $2::vector 
		LIMIT $3
	`
	rows, err := r.pool.Query(ctx, query, storyID, vecStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (r *postgresRepo) SearchStoriesByEmbedding(ctx context.Context, embedding []float32, limit int) ([]uuid.UUID, error) {
	vecStr := vecToString(embedding)

	query := `
		SELECT story_id 
		FROM story_lorebooks 
		WHERE embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector 
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, query, vecStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (r *postgresRepo) InsertCanonFact(ctx context.Context, fact models.CanonFact) error {
	query := `
		INSERT INTO canon_lorebooks (id, fandom_slug, entity_name, fact_text, embedding)
		VALUES ($1, $2, $3, $4, $5::vector)
	`
	vecStr := vecToString(fact.Embedding)
	_, err := r.pool.Exec(ctx, query, fact.ID, fact.FandomSlug, fact.EntityName, fact.FactText, vecStr)
	return err
}

// GetGlobalCanonFacts вытаскивает все факты, которые помечены как глобальные правила
func (r *postgresRepo) GetGlobalCanonFacts(ctx context.Context, fandomSlug string) ([]string, error) {
	query := `
		SELECT fact_text 
		FROM canon_lorebooks 
		WHERE fandom_slug = $1 AND is_global_rule = true
	`
	rows, err := r.pool.Query(ctx, query, fandomSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []string
	for rows.Next() {
		var fact string
		if err := rows.Scan(&fact); err == nil {
			facts = append(facts, fact)
		}
	}
	return facts, nil
}

// GetCanonFactsByEntities вытаскивает факты о конкретных персонажах и объектах
func (r *postgresRepo) GetCanonFactsByEntities(ctx context.Context, fandomSlug string, entities []string) ([]string, error) {
	if len(entities) == 0 {
		return nil, nil
	}
	query := `
		SELECT fact_text 
		FROM canon_lorebooks 
		WHERE fandom_slug = $1 AND LOWER(entity_name) = ANY($2)
	`
	rows, err := r.pool.Query(ctx, query, fandomSlug, entities)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []string
	for rows.Next() {
		var fact string
		if err := rows.Scan(&fact); err == nil {
			facts = append(facts, fact)
		}
	}
	return facts, nil
}
