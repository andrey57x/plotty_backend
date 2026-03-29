package main

import (
	"context"
	"net/http"

	"github.com/fivecode/plotty/core/app"
	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/redis"
	"github.com/fivecode/plotty/core/config"
	"github.com/fivecode/plotty/internal/infrastructure/postgres"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/rs/zerolog/log"
)

func main() {
	logger.Init()

	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to rabbitmq")
	}
	defer rmqConn.Close()

	rmqChan, err := rmqConn.Channel()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create rabbitmq channel")
	}
	defer rmqChan.Close()

	rmqChan.QueueDeclare("ml_tasks_queue", true, false, false, false, nil)
	rmqChan.QueueDeclare("spellcheck_queue", true, false, false, false, nil)
	rmqChan.QueueDeclare("ml_results_queue", true, false, false, false, nil)

	if err := postgres.RunMigrations(cfg.GetDSN(), "migrations"); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}

	pool, err := postgres.NewPostgresPool(ctx, cfg.GetDSN())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer pool.Close()

	redisDB, err := redis.NewRedisDB(cfg.GetRedisAddr(), cfg.RedisPassword)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer redisDB.Close()

	h := app.NewHTTPHandler(cfg, pool, redisDB, rmqChan)

	addr := ":" + cfg.HTTPPort
	log.Info().Str("addr", addr).Msg("starting HTTP server")
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal().Err(err).Msg("server error")
	}
}
