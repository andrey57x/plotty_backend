package main

import (
	"context"
	"log"
	"net/http"

	"github.com/fivecode/plotty/core/app"
	"github.com/fivecode/plotty/internal/config"
	"github.com/fivecode/plotty/internal/infrastructure"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := infrastructure.RunMigrations(cfg.GetDSN(), "migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	pool, err := infrastructure.NewPostgresPool(ctx, cfg.GetDSN())
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	h := app.NewHTTPHandler(cfg, pool)
	addr := ":" + cfg.HTTPPort
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, h))
}
