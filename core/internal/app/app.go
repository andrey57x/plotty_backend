package app

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type App struct {
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Router *chi.Mux
}

func NewApp(db *pgxpool.Pool, rdb *redis.Client) *App {
	r := chi.NewRouter()

	// Базовые перехватчики
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	return &App{
		DB:     db,
		Redis:  rdb,
		Router: r,
	}
}
