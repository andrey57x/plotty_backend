package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fivecode/plotty/internal/infrastructure/postgres"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/fivecode/plotty/ml/app"
	"github.com/fivecode/plotty/ml/config"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфига: %v", err)
	}

	if err := postgres.RunMigrations(cfg.GetDSN(), "migrations/ml"); err != nil {
		log.Fatalf("migrations error: %v", err)
	}

	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Ошибка подключения к RabbitMQ: %v", err)
	}

	dbPool, err := postgres.NewPostgresPool(ctx, cfg.GetDSN())
	if err != nil {
		log.Fatalf("Ошибка БД: %v", err)
	}

	application, err := app.NewApp(cfg, rmqConn, dbPool)
	if err != nil {
		log.Fatalf("Ошибка инициализации ML приложения: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := application.Run(ctx); err != nil {
			log.Printf("Остановка воркера: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Получен сигнал завершения. Выключаем ML Worker...")

	cancel()
	application.Stop()

	log.Println("ML Worker успешно остановлен.")
}
