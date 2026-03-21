package main

import (
	"context"
	"log"
	"net/http"

	"github.com/fivecode/plotty/core/app"
	"github.com/fivecode/plotty/internal/config"
	"github.com/fivecode/plotty/internal/infrastructure"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	rmqConn, err := rabbitmq.NewConnection(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer rmqConn.Close()

	rmqChan, err := rmqConn.Channel()
	if err != nil {
		log.Fatalf("rabbitmq channel: %v", err)
	}
	defer rmqChan.Close()

	rmqChan.QueueDeclare("ml_tasks_queue", true, false, false, false, nil)
	rmqChan.QueueDeclare("ml_results_queue", true, false, false, false, nil)

	if err := infrastructure.RunMigrations(cfg.GetDSN(), "migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	pool, err := infrastructure.NewPostgresPool(ctx, cfg.GetDSN())
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	h := app.NewHTTPHandler(cfg, pool, rmqChan)

	addr := ":" + cfg.HTTPPort
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, h))
}
