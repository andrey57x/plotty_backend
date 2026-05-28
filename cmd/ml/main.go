package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/internal/infrastructure/postgres"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/fivecode/plotty/ml/app"
	"github.com/fivecode/plotty/ml/config"
	"github.com/rs/zerolog/log"
)

func main() {
	logger.Init() // Инициализация логгера zerolog

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Ошибка загрузки конфига")
	}

	if err := postgres.RunMigrations(cfg.GetDSN(), "migrations/ml"); err != nil {
		log.Fatal().Err(err).Msg("Ошибка миграций")
	}

	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Ошибка подключения к RabbitMQ")
	}

	dbPool, err := postgres.NewPostgresPool(ctx, cfg.GetDSN())
	if err != nil {
		log.Fatal().Err(err).Msg("Ошибка БД")
	}

	application, err := app.NewApp(cfg, rmqConn, dbPool)
	if err != nil {
		log.Fatal().Err(err).Msg("Ошибка инициализации ML приложения")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := application.Run(ctx); err != nil {
			log.Info().Err(err).Msg("Остановка воркера")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Получен сигнал завершения. Выключаем ML Worker...")

	cancel()
	application.Stop()

	log.Info().Msg("ML Worker успешно остановлен.")
}
