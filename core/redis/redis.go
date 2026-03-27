package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/fivecode/plotty/core/logger"
	namederrors "github.com/fivecode/plotty/core/named_errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RedisDB struct {
	Client *redis.Client
}

func NewRedisDB(addr, password string) (*RedisDB, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &RedisDB{Client: client}, nil
}

func (r *RedisDB) Close() error {
	return r.Client.Close()
}

func (r *RedisDB) CreateSession(ctx context.Context, userID uint64, duration time.Duration) (string, error) {
	log := logger.FromContext(ctx)
	sessionID := uuid.NewString()
	key := "session:" + sessionID

	err := r.Client.Set(ctx, key, userID, duration).Err()
	if err != nil {
		log.Error().Err(err).Msg("failed to set session in redis")
		return "", err
	}

	return sessionID, nil
}

func (r *RedisDB) GetUserIDBySession(ctx context.Context, sessionID string) (uint64, error) {
	log := logger.FromContext(ctx)
	key := "session:" + sessionID

	val, err := r.Client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		log.Warn().Str("key", key).Msg("session not found in redis")
		return 0, namederrors.ErrInvalidSession
	}
	if err != nil {
		log.Error().Err(err).Str("key", key).Msg("failed to get session from redis")
		return 0, err
	}

	userID, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		log.Error().Err(err).Str("value", val).Msg("failed to parse userID from redis session value")
		return 0, fmt.Errorf("failed to parse userID from session: %w", err)
	}

	return userID, nil
}

func (r *RedisDB) DeleteSession(ctx context.Context, sessionID string) error {
	log := logger.FromContext(ctx)
	key := "session:" + sessionID

	err := r.Client.Del(ctx, key).Err()
	if err != nil {
		log.Error().Err(err).Str("key", key).Msg("failed to delete session from redis")
		return err
	}
	return nil
}
