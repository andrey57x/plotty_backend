package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, s models.Story, tagIDs []uuid.UUID) (*models.Story, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO stories (id, slug, title, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, s.ID, s.Slug, s.Title, now, now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, named_errors.ErrConflict
		}
		return nil, err
	}
	for _, tid := range tagIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO story_tags (story_id, tag_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, s.ID, tid); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.CreatedAt = now
	s.UpdatedAt = now
	return &s, nil
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, title *string, tagIDs *[]uuid.UUID) (*models.Story, error) {
	cur, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	newTitle := cur.Title
	if title != nil {
		newTitle = *title
	}
	now := time.Now().UTC()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		UPDATE stories SET title = $2, updated_at = $3
		WHERE id = $1
	`, id, newTitle, now)
	if err != nil {
		return nil, err
	}
	if tagIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM story_tags WHERE story_id = $1`, id); err != nil {
			return nil, err
		}
		for _, tid := range *tagIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO story_tags (story_id, tag_id) VALUES ($1, $2)
			`, id, tid); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	cur.Title = newTitle
	cur.UpdatedAt = now
	return cur, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Story, error) {
	var s models.Story
	err := r.pool.QueryRow(ctx, `
		SELECT id, slug, title, created_at, updated_at
		FROM stories WHERE id = $1
	`, id).Scan(&s.ID, &s.Slug, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*models.Story, error) {
	var s models.Story
	err := r.pool.QueryRow(ctx, `
		SELECT id, slug, title, created_at, updated_at
		FROM stories WHERE slug = $1
	`, slug).Scan(&s.ID, &s.Slug, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM stories WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	return nil
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func (r *Repository) groupTagSlugsByCategory(ctx context.Context, tagSlugs []string) (map[string][]string, int, error) {
	tagSlugs = dedupeStrings(tagSlugs)
	if len(tagSlugs) == 0 {
		return map[string][]string{}, 0, nil
	}

	type row struct {
		category string
		slug     string
	}
	rows, err := r.pool.Query(ctx, `
		SELECT category, slug
		FROM tags
		WHERE slug = ANY($1::text[])
	`, tagSlugs)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make(map[string][]string)
	found := 0
	for rows.Next() {
		var rr row
		if err := rows.Scan(&rr.category, &rr.slug); err != nil {
			return nil, 0, err
		}
		out[rr.category] = append(out[rr.category], rr.slug)
		found++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, found, nil
}

func (r *Repository) ListIDs(ctx context.Context, q string, tagSlugs []string, limit, offset int) ([]uuid.UUID, error) {
	tagSlugs = dedupeStrings(tagSlugs)
	groups, found, err := r.groupTagSlugsByCategory(ctx, tagSlugs)
	if err != nil {
		return nil, err
	}
	if len(tagSlugs) > 0 && found != len(tagSlugs) {
		return []uuid.UUID{}, nil
	}

	args := []any{q, limit, offset}
	whereTags := ""
	if len(groups) > 0 {
		i := 0
		for _, slugs := range groups {
			i++
			args = append(args, slugs)
			ph := len(args)
			whereTags += fmt.Sprintf(`
				AND EXISTS (
					SELECT 1
					FROM story_tags st
					JOIN tags tg ON tg.id = st.tag_id
					WHERE st.story_id = s.id AND tg.slug = ANY($%d::text[])
				)
			`, ph)
		}
	}

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT s.id
		FROM stories s
		WHERE ($1 = '' OR s.title ILIKE '%%' || $1 || '%%')
		%s
		ORDER BY s.updated_at DESC
		LIMIT $2 OFFSET $3
	`, whereTags), args...)
	if err != nil {
		return nil, err
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

func (r *Repository) CountList(ctx context.Context, q string, tagSlugs []string) (int, error) {
	tagSlugs = dedupeStrings(tagSlugs)
	groups, found, err := r.groupTagSlugsByCategory(ctx, tagSlugs)
	if err != nil {
		return 0, err
	}
	if len(tagSlugs) > 0 && found != len(tagSlugs) {
		return 0, nil
	}

	args := []any{q}
	whereTags := ""
	if len(groups) > 0 {
		for _, slugs := range groups {
			args = append(args, slugs)
			ph := len(args)
			whereTags += fmt.Sprintf(`
				AND EXISTS (
					SELECT 1
					FROM story_tags st
					JOIN tags tg ON tg.id = st.tag_id
					WHERE st.story_id = s.id AND tg.slug = ANY($%d::text[])
				)
			`, ph)
		}
	}

	var total int
	err = r.pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)::int
		FROM stories s
		WHERE ($1 = '' OR s.title ILIKE '%%' || $1 || '%%')
		%s
	`, whereTags), args...).Scan(&total)
	return total, err
}

func (r *Repository) LoadStoriesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]models.Story, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]models.Story{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, slug, title, created_at, updated_at
		FROM stories WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[uuid.UUID]models.Story, len(ids))
	for rows.Next() {
		var s models.Story
		if err := rows.Scan(&s.ID, &s.Slug, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		m[s.ID] = s
	}
	return m, rows.Err()
}

func (r *Repository) TagsForStories(ctx context.Context, storyIDs []uuid.UUID) (map[uuid.UUID][]models.Tag, error) {
	out := make(map[uuid.UUID][]models.Tag)
	if len(storyIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT st.story_id, t.id, t.category, t.slug, t.name
		FROM story_tags st
		JOIN tags t ON t.id = st.tag_id
		WHERE st.story_id = ANY($1)
		ORDER BY t.category ASC, t.name ASC
	`, storyIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var t models.Tag
		if err := rows.Scan(&sid, &t.ID, &t.Category, &t.Slug, &t.Name); err != nil {
			return nil, err
		}
		out[sid] = append(out[sid], t)
	}
	return out, rows.Err()
}

func (r *Repository) ChapterCounts(ctx context.Context, storyIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	out := make(map[uuid.UUID]int)
	if len(storyIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT story_id, COUNT(*)::int FROM chapters WHERE story_id = ANY($1) GROUP BY story_id
	`, storyIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var c int
		if err := rows.Scan(&sid, &c); err != nil {
			return nil, err
		}
		out[sid] = c
	}
	return out, rows.Err()
}

func (r *Repository) TagsForStory(ctx context.Context, storyID uuid.UUID) ([]models.Tag, error) {
	m, err := r.TagsForStories(ctx, []uuid.UUID{storyID})
	if err != nil {
		return nil, err
	}
	return m[storyID], nil
}
