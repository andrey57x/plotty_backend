package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/models"
	namederrors "github.com/fivecode/plotty/core/named_errors"
	"github.com/fivecode/plotty/core/redis"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthRepository struct {
	Pool  *pgxpool.Pool
	Redis *redis.RedisDB
}

func New(pool *pgxpool.Pool, redisDB *redis.RedisDB) *AuthRepository {
	return &AuthRepository{
		Pool:  pool,
		Redis: redisDB,
	}
}

func (r *AuthRepository) CreateSession(ctx context.Context, userID uint64) (string, error) {
	log := logger.FromContext(ctx)
	log.Info().Uint64("user_id", userID).Msg("creating session via redis store")
	return r.Redis.CreateSession(ctx, userID, 30*24*time.Hour)
}

func (r *AuthRepository) DeleteSession(ctx context.Context, sessionID string) error {
	log := logger.FromContext(ctx)
	log.Info().Str("session_id", sessionID).Msg("deleting session via redis store")
	return r.Redis.DeleteSession(ctx, sessionID)
}

func (r *AuthRepository) GetUserIDBySession(ctx context.Context, sessionID string) (uint64, error) {
	log := logger.FromContext(ctx)
	log.Info().Str("session_id", sessionID).Msg("getting user id by session via redis store")
	return r.Redis.GetUserIDBySession(ctx, sessionID)
}

func (r *AuthRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	log := logger.FromContext(ctx)
	log.Info().Str("email", email).Msg("getting user by email from PostgreSQL")

	query := `SELECT id, email, password_hash, username, avatar_url, created_at, updated_at FROM users WHERE email = $1`

	user := &models.User{}
	var avatarURL sql.NullString
	var updatedAt sql.NullTime

	err := r.Pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.Username,
		&avatarURL,
		&user.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn().Str("email", email).Msg("user not found by email")
			return nil, namederrors.ErrNotFound
		}
		log.Error().Err(err).Str("email", email).Msg("failed to query user")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	if updatedAt.Valid {
		user.UpdatedAt = &updatedAt.Time
	}

	return user, nil
}

func (r *AuthRepository) CreateUser(ctx context.Context, email, passwordHash string) (*models.User, error) {
	log := logger.FromContext(ctx)
	log.Info().Str("email", email).Msg("creating user in PostgreSQL")

	username := strings.Split(email, "@")[0]

	query := `
		INSERT INTO users (email, password_hash, username)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, username, avatar_url, created_at, updated_at
	`

	user := &models.User{}
	var avatarURL sql.NullString
	var updatedAt sql.NullTime

	err := r.Pool.QueryRow(ctx, query, email, passwordHash, username).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.Username,
		&avatarURL,
		&user.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			log.Warn().Str("email", email).Msg("user already exists")
			return nil, namederrors.ErrUserExists
		}
		log.Error().Err(err).Msg("failed to create user in PostgreSQL")
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	if updatedAt.Valid {
		user.UpdatedAt = &updatedAt.Time
	}

	log.Info().Uint64("user_id", user.ID).Msg("user created in PostgreSQL")
	return user, nil
}

func (r *AuthRepository) UpdateUser(ctx context.Context, userID uint64, username *string, avatarURL *string) (*models.User, error) {
	log := logger.FromContext(ctx)
	log.Info().Uint64("user_id", userID).Msg("updating user profile")

	cur, err := r.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	newUsername := cur.Username
	if username != nil {
		newUsername = *username
	}
	newAvatar := cur.AvatarURL
	if avatarURL != nil {
		if *avatarURL == "" {
			newAvatar = nil
		} else {
			newAvatar = avatarURL
		}
	}

	_, err = r.Pool.Exec(ctx, `
		UPDATE users SET username = $2, avatar_url = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, userID, newUsername, newAvatar)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return nil, namederrors.ErrConflict
		}
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return r.GetUserByID(ctx, userID)
}

func (r *AuthRepository) GetUserByID(ctx context.Context, userID uint64) (*models.User, error) {
	log := logger.FromContext(ctx)
	log.Info().Uint64("user_id", userID).Msg("getting user by id from PostgreSQL")

	query := `SELECT id, email, password_hash, username, avatar_url, created_at, updated_at FROM users WHERE id = $1`

	user := &models.User{}
	var avatarURL sql.NullString
	var updatedAt sql.NullTime

	err := r.Pool.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.Username,
		&avatarURL,
		&user.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Warn().Uint64("user_id", userID).Msg("user not found by id")
			return nil, namederrors.ErrNotFound
		}
		log.Error().Err(err).Uint64("user_id", userID).Msg("failed to query user")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	if updatedAt.Valid {
		user.UpdatedAt = &updatedAt.Time
	}

	return user, nil
}
