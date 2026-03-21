package app

import (
	"net/http"

	"github.com/fivecode/plotty/core/router"
	"github.com/fivecode/plotty/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewHTTPHandler(cfg *config.Config, db *pgxpool.Pool) http.Handler {
	return router.NewRouter(cfg, db)
}
