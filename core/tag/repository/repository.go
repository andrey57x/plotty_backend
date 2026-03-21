package repository

import (
	"context"
	"errors"
	"strings"

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

func (r *Repository) List(ctx context.Context, category string) ([]models.Tag, error) {
	category = strings.TrimSpace(category)
	var rows pgx.Rows
	var err error
	if category == "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, category, slug, name
			FROM tags
			ORDER BY category ASC, name ASC
		`)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, category, slug, name
			FROM tags
			WHERE category = $1
			ORDER BY name ASC
		`, category)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Tag
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Category, &t.Slug, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*models.Tag, error) {
	var t models.Tag
	err := r.pool.QueryRow(ctx, `
		SELECT id, category, slug, name FROM tags WHERE id = $1
	`, id).Scan(&t.ID, &t.Category, &t.Slug, &t.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, named_errors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) ValidateAllExist(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tags WHERE id = ANY($1)`, ids).Scan(&n)
	if err != nil {
		return err
	}
	if n != len(ids) {
		return named_errors.ErrInvalidInput
	}
	return nil
}
