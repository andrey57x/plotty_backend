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
	log := logger.FromContext(ctx)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Msg("story_repo: failed to begin tx for create")
		return nil, fmt.Errorf("story_repo.Create begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO stories (id, slug, title, status, author_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, s.ID, s.Slug, s.Title, "draft", s.AuthorID, now, now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, named_errors.ErrConflict
		}
		log.Error().Err(err).Stringer("story_id", s.ID).Msg("story_repo: insert failed")
		return nil, fmt.Errorf("story_repo.Create insert: %w", err)
	}
	for _, tid := range tagIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO story_tags (story_id, tag_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, s.ID, tid); err != nil {
			log.Error().Err(err).Stringer("story_id", s.ID).Stringer("tag_id", tid).Msg("story_repo: insert tag failed")
			return nil, fmt.Errorf("story_repo.Create insert tag: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("story_repo: commit failed")
		return nil, fmt.Errorf("story_repo.Create commit: %w", err)
	}
	s.Status = "draft"
	s.CreatedAt = now
	s.UpdatedAt = now

	log.Info().Stringer("story_id", s.ID).Str("slug", s.Slug).Msg("story_repo: created")
	return &s, nil
}

func (r *Repository) Update(ctx context.Context, id uuid.UUID, title *string, tagIDs *[]uuid.UUID) (*models.Story, error) {
	log := logger.FromContext(ctx)

	cur, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("story_repo.Update get current: %w", err)
	}
	newTitle := cur.Title
	if title != nil {
		newTitle = *title
	}
	now := time.Now().UTC()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", id).Msg("story_repo: failed to begin tx for update")
		return nil, fmt.Errorf("story_repo.Update begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		UPDATE stories SET title = $2, updated_at = $3
		WHERE id = $1
	`, id, newTitle, now)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", id).Msg("story_repo: update failed")
		return nil, fmt.Errorf("story_repo.Update exec: %w", err)
	}
	if tagIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM story_tags WHERE story_id = $1`, id); err != nil {
			log.Error().Err(err).Stringer("story_id", id).Msg("story_repo: delete old tags failed")
			return nil, fmt.Errorf("story_repo.Update delete tags: %w", err)
		}
		for _, tid := range *tagIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO story_tags (story_id, tag_id) VALUES ($1, $2)
			`, id, tid); err != nil {
				log.Error().Err(err).Stringer("story_id", id).Stringer("tag_id", tid).Msg("story_repo: insert tag failed")
				return nil, fmt.Errorf("story_repo.Update insert tag: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Stringer("story_id", id).Msg("story_repo: commit failed")
		return nil, fmt.Errorf("story_repo.Update commit: %w", err)
	}
	cur.Title = newTitle
	cur.UpdatedAt = now

	log.Info().Stringer("story_id", id).Msg("story_repo: updated")
	return cur, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Story, error) {
	var s models.Story
	err := r.pool.QueryRow(ctx, `
		SELECT id, slug, title, status, author_id, ai_summary, created_at, updated_at
		FROM stories WHERE id = $1
	`, id).Scan(&s.ID, &s.Slug, &s.Title, &s.Status, &s.AuthorID, &s.AiSummary, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("story_id", id).Msg("story_repo: GetByID query failed")
		return nil, fmt.Errorf("story_repo.GetByID: %w", err)
	}
	return &s, nil
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*models.Story, error) {
	var s models.Story
	err := r.pool.QueryRow(ctx, `
		SELECT id, slug, title, status, author_id, ai_summary, created_at, updated_at
		FROM stories WHERE slug = $1
	`, slug).Scan(&s.ID, &s.Slug, &s.Title, &s.Status, &s.AuthorID, &s.AiSummary, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("slug", slug).Msg("story_repo: GetBySlug query failed")
		return nil, fmt.Errorf("story_repo.GetBySlug: %w", err)
	}
	return &s, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM stories WHERE id = $1`, id)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("story_id", id).Msg("story_repo: delete failed")
		return fmt.Errorf("story_repo.Delete: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return named_errors.ErrNotFound
	}
	logger.Ctx(ctx).Info().Stringer("story_id", id).Msg("story_repo: deleted")
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
		return nil, 0, fmt.Errorf("story_repo.groupTagSlugsByCategory: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]string)
	found := 0
	for rows.Next() {
		var rr row
		if err := rows.Scan(&rr.category, &rr.slug); err != nil {
			return nil, 0, fmt.Errorf("story_repo.groupTagSlugsByCategory scan: %w", err)
		}
		out[rr.category] = append(out[rr.category], rr.slug)
		found++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("story_repo.groupTagSlugsByCategory rows: %w", err)
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
		AND s.status = 'published'
		%s
		ORDER BY s.updated_at DESC
		LIMIT $2 OFFSET $3
	`, whereTags), args...)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("story_repo: ListIDs query failed")
		return nil, fmt.Errorf("story_repo.ListIDs: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("story_repo.ListIDs scan: %w", err)
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
		AND s.status = 'published'
		%s
	`, whereTags), args...).Scan(&total)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("story_repo: CountList failed")
		return 0, fmt.Errorf("story_repo.CountList: %w", err)
	}
	return total, nil
}

func (r *Repository) ListMyIDs(ctx context.Context, userID uint64, q string, tagSlugs []string, limit, offset int) ([]uuid.UUID, error) {
	tagSlugs = dedupeStrings(tagSlugs)
	groups, found, err := r.groupTagSlugsByCategory(ctx, tagSlugs)
	if err != nil {
		return nil, err
	}
	if len(tagSlugs) > 0 && found != len(tagSlugs) {
		return []uuid.UUID{}, nil
	}

	args := []any{userID, q, limit, offset}
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

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT s.id
		FROM stories s
		WHERE s.author_id = $1
		AND ($2 = '' OR s.title ILIKE '%%' || $2 || '%%')
		%s
		ORDER BY s.updated_at DESC
		LIMIT $3 OFFSET $4
	`, whereTags), args...)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Uint64("user_id", userID).Msg("story_repo: ListMyIDs query failed")
		return nil, fmt.Errorf("story_repo.ListMyIDs: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("story_repo.ListMyIDs scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) CountMyList(ctx context.Context, userID uint64, q string, tagSlugs []string) (int, error) {
	tagSlugs = dedupeStrings(tagSlugs)
	groups, found, err := r.groupTagSlugsByCategory(ctx, tagSlugs)
	if err != nil {
		return 0, err
	}
	if len(tagSlugs) > 0 && found != len(tagSlugs) {
		return 0, nil
	}

	args := []any{userID, q}
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
		WHERE s.author_id = $1
		AND ($2 = '' OR s.title ILIKE '%%' || $2 || '%%')
		%s
	`, whereTags), args...).Scan(&total)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Uint64("user_id", userID).Msg("story_repo: CountMyList failed")
		return 0, fmt.Errorf("story_repo.CountMyList: %w", err)
	}
	return total, nil
}

func (r *Repository) LoadStoriesByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]models.Story, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]models.Story{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, slug, title, status, author_id, ai_summary, created_at, updated_at
		FROM stories WHERE id = ANY($1)
	`, ids)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("story_repo: LoadStoriesByIDs query failed")
		return nil, fmt.Errorf("story_repo.LoadStoriesByIDs: %w", err)
	}
	defer rows.Close()
	m := make(map[uuid.UUID]models.Story, len(ids))
	for rows.Next() {
		var s models.Story
		if err := rows.Scan(&s.ID, &s.Slug, &s.Title, &s.Status, &s.AuthorID, &s.AiSummary, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("story_repo.LoadStoriesByIDs scan: %w", err)
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
		return nil, fmt.Errorf("story_repo.TagsForStories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var t models.Tag
		if err := rows.Scan(&sid, &t.ID, &t.Category, &t.Slug, &t.Name); err != nil {
			return nil, fmt.Errorf("story_repo.TagsForStories scan: %w", err)
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
		return nil, fmt.Errorf("story_repo.ChapterCounts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var c int
		if err := rows.Scan(&sid, &c); err != nil {
			return nil, fmt.Errorf("story_repo.ChapterCounts scan: %w", err)
		}
		out[sid] = c
	}
	return out, rows.Err()
}

func (r *Repository) ChapterCountsPublished(ctx context.Context, storyIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	out := make(map[uuid.UUID]int)
	if len(storyIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT story_id, COUNT(*)::int
		FROM chapters
		WHERE story_id = ANY($1) AND status = 'published'
		GROUP BY story_id
	`, storyIDs)
	if err != nil {
		return nil, fmt.Errorf("story_repo.ChapterCountsPublished: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var c int
		if err := rows.Scan(&sid, &c); err != nil {
			return nil, fmt.Errorf("story_repo.ChapterCountsPublished scan: %w", err)
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

func (r *Repository) LikeCounts(ctx context.Context, storyIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	out := make(map[uuid.UUID]int)
	if len(storyIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT story_id, COUNT(*)::int FROM story_likes WHERE story_id = ANY($1) GROUP BY story_id
	`, storyIDs)
	if err != nil {
		return nil, fmt.Errorf("story_repo.LikeCounts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var c int
		if err := rows.Scan(&sid, &c); err != nil {
			return nil, fmt.Errorf("story_repo.LikeCounts scan: %w", err)
		}
		out[sid] = c
	}
	return out, rows.Err()
}

func (r *Repository) LikeCount(ctx context.Context, storyID uuid.UUID) (int, error) {
	var c int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*)::int FROM story_likes WHERE story_id = $1`, storyID).Scan(&c)
	if err != nil {
		return 0, fmt.Errorf("story_repo.LikeCount: %w", err)
	}
	return c, nil
}

func (r *Repository) IsLikedByUser(ctx context.Context, storyID uuid.UUID, userID uint64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM story_likes WHERE story_id = $1 AND user_id = $2)
	`, storyID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("story_repo.IsLikedByUser: %w", err)
	}
	return exists, nil
}

func (r *Repository) AuthorsForStories(ctx context.Context, storyIDs []uuid.UUID) (map[uuid.UUID]*models.StoryAuthor, error) {
	out := make(map[uuid.UUID]*models.StoryAuthor)
	if len(storyIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, u.id, u.username, u.avatar_url
		FROM stories s
		JOIN users u ON u.id = s.author_id
		WHERE s.id = ANY($1) AND s.author_id IS NOT NULL
	`, storyIDs)
	if err != nil {
		return nil, fmt.Errorf("story_repo.AuthorsForStories: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var a models.StoryAuthor
		var avatar *string
		if err := rows.Scan(&sid, &a.ID, &a.Username, &avatar); err != nil {
			return nil, fmt.Errorf("story_repo.AuthorsForStories scan: %w", err)
		}
		a.AvatarURL = avatar
		out[sid] = &a
	}
	return out, rows.Err()
}

func (r *Repository) GetAuthorForStory(ctx context.Context, storyID uuid.UUID) (*models.StoryAuthor, error) {
	m, err := r.AuthorsForStories(ctx, []uuid.UUID{storyID})
	if err != nil {
		return nil, err
	}
	return m[storyID], nil
}

func (r *Repository) Publish(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE stories SET status = 'published', updated_at = $2 WHERE id = $1 AND status = 'draft'`, id, time.Now().UTC())
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("story_id", id).Msg("story_repo: publish failed")
		return fmt.Errorf("story_repo.Publish: %w", err)
	}
	return nil
}

func (r *Repository) UpdateAISummary(ctx context.Context, storyID uuid.UUID, summary string) error {
	_, err := r.pool.Exec(ctx, `UPDATE stories SET ai_summary = $2 WHERE id = $1`, storyID, summary)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("story_id", storyID).Msg("story_repo: update ai_summary failed")
		return fmt.Errorf("story_repo.UpdateAISummary: %w", err)
	}
	return nil
}

func (r *Repository) ListSemanticIDs(ctx context.Context, semanticIDs []uuid.UUID, tagSlugs []string, limit, offset int) ([]uuid.UUID, error) {
	tagSlugs = dedupeStrings(tagSlugs)
	groups, found, err := r.groupTagSlugsByCategory(ctx, tagSlugs)
	if err != nil {
		return nil, err
	}
	if len(tagSlugs) > 0 && found != len(tagSlugs) {
		return []uuid.UUID{}, nil
	}

	args := []any{semanticIDs, limit, offset}
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

	query := fmt.Sprintf(`
		SELECT s.id
		FROM unnest($1::uuid[]) WITH ORDINALITY t(id, ord)
		JOIN stories s ON s.id = t.id
		WHERE s.status = 'published'
		%s
		ORDER BY t.ord
		LIMIT $2 OFFSET $3
	`, whereTags)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("story_repo.ListSemanticIDs: %w", err)
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

func (r *Repository) CountSemanticIDs(ctx context.Context, semanticIDs []uuid.UUID, tagSlugs []string) (int, error) {
	tagSlugs = dedupeStrings(tagSlugs)
	groups, found, err := r.groupTagSlugsByCategory(ctx, tagSlugs)
	if err != nil {
		return 0, err
	}
	if len(tagSlugs) > 0 && found != len(tagSlugs) {
		return 0, nil
	}

	args := []any{semanticIDs}
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

	query := fmt.Sprintf(`
		SELECT COUNT(*)::int
		FROM unnest($1::uuid[]) t(id)
		JOIN stories s ON s.id = t.id
		WHERE s.status = 'published'
		%s
	`, whereTags)

	var total int
	err = r.pool.QueryRow(ctx, query, args...).Scan(&total)
	return total, err
}
