package app

import (
	"net/http"

	"github.com/fivecode/plotty/core/router"
	"github.com/fivecode/plotty/core/redis"
	"github.com/fivecode/plotty/core/config"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

func NewHTTPHandler(cfg *config.Config, db *pgxpool.Pool, redisDB *redis.RedisDB, rmqChan *amqp.Channel) http.Handler {
	return router.NewRouter(cfg, db, redisDB, rmqChan)
}
