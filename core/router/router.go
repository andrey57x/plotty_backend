package router

import (
	"context"
	"encoding/json"
	"net/http"

	aideliv "github.com/fivecode/plotty/core/ai/delivery"
	airepo "github.com/fivecode/plotty/core/ai/repository"
	aiuc "github.com/fivecode/plotty/core/ai/usecase"
	chdeliv "github.com/fivecode/plotty/core/chapter/delivery"
	chrepo "github.com/fivecode/plotty/core/chapter/repository"
	chuc "github.com/fivecode/plotty/core/chapter/usecase"
	storydeliv "github.com/fivecode/plotty/core/story/delivery"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	storyuc "github.com/fivecode/plotty/core/story/usecase"
	tagdeliv "github.com/fivecode/plotty/core/tag/delivery"
	tagrepo "github.com/fivecode/plotty/core/tag/repository"
	taguc "github.com/fivecode/plotty/core/tag/usecase"
	"github.com/fivecode/plotty/internal/config"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

const uuidRe = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

func NewRouter(cfg *config.Config, pool *pgxpool.Pool, rmqChan *amqp.Channel) http.Handler {
	tr := tagrepo.New(pool)
	sr := storyrepo.New(pool)
	cr := chrepo.New(pool)
	ar := airepo.New(pool)

	tu := taguc.New(tr)
	su := storyuc.New(sr, tr, cr)
	cu := chuc.New(cr, sr)

	au := aiuc.New(ar, cr, rmqChan)

	sd := storydeliv.New(su)
	cd := chdeliv.New(cu)
	td := tagdeliv.New(tu)
	ad := aideliv.New(au)

	go func() {
		msgs, err := rmqChan.Consume("ml_results_queue", "core_worker", false, false, false, false, nil)
		if err == nil {
			for msg := range msgs {
				var res rabbitmq.MLResultMessage
				if err := json.Unmarshal(msg.Body, &res); err == nil {
					_ = au.ProcessMLResult(context.Background(), res)
				}
				msg.Ack(false)
			}
		}
	}()

	r := mux.NewRouter()

	r.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}).Methods(http.MethodGet)

	r.HandleFunc("/stories", sd.List).Methods(http.MethodGet)
	r.HandleFunc("/stories", sd.Create).Methods(http.MethodPost)
	r.HandleFunc("/stories/{id:"+uuidRe+"}", sd.Patch).Methods(http.MethodPatch)
	r.HandleFunc("/stories/{id:"+uuidRe+"}", sd.Delete).Methods(http.MethodDelete)
	r.HandleFunc("/stories/{storyId:"+uuidRe+"}/chapters", cd.CreateUnderStory).Methods(http.MethodPost)
	r.HandleFunc("/stories/{slug}", sd.GetBySlug).Methods(http.MethodGet)

	r.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Get).Methods(http.MethodGet)
	r.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Patch).Methods(http.MethodPatch)
	r.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Delete).Methods(http.MethodDelete)

	r.HandleFunc("/tags", td.List).Methods(http.MethodGet)

	r.HandleFunc("/ai/spellcheck", ad.Spellcheck).Methods(http.MethodPost)
	r.HandleFunc("/ai/image-generation", ad.ImageGeneration).Methods(http.MethodPost)
	r.HandleFunc("/ai/jobs/{jobId:"+uuidRe+"}", ad.GetJob).Methods(http.MethodGet)

	return corsMiddleware(r, cfg.AllowedOrigins)
}

func corsMiddleware(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
