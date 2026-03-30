package router

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	aideliv "github.com/fivecode/plotty/core/ai/delivery"
	airepo "github.com/fivecode/plotty/core/ai/repository"
	aiuc "github.com/fivecode/plotty/core/ai/usecase"
	authdeliv "github.com/fivecode/plotty/core/auth/delivery"
	authrepo "github.com/fivecode/plotty/core/auth/repository"
	authuc "github.com/fivecode/plotty/core/auth/usecase"
	chdeliv "github.com/fivecode/plotty/core/chapter/delivery"
	chrepo "github.com/fivecode/plotty/core/chapter/repository"
	chuc "github.com/fivecode/plotty/core/chapter/usecase"
	"github.com/fivecode/plotty/core/middleware"
	storydeliv "github.com/fivecode/plotty/core/story/delivery"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	storyuc "github.com/fivecode/plotty/core/story/usecase"
	"github.com/fivecode/plotty/core/redis"
	tagdeliv "github.com/fivecode/plotty/core/tag/delivery"
	tagrepo "github.com/fivecode/plotty/core/tag/repository"
	taguc "github.com/fivecode/plotty/core/tag/usecase"
	"github.com/fivecode/plotty/core/config"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
)

const uuidRe = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

func NewRouter(cfg *config.Config, pool *pgxpool.Pool, redisDB *redis.RedisDB, rmqChan *amqp.Channel) http.Handler {
	tr := tagrepo.New(pool)
	sr := storyrepo.New(pool)
	cr := chrepo.New(pool)
	ar := airepo.New(pool)
	authr := authrepo.New(pool, redisDB)

	tu := taguc.New(tr)
	su := storyuc.New(sr, tr, cr)
	cu := chuc.New(cr, sr)
	au := aiuc.New(ar, cr, rmqChan)
	authu := authuc.New(authr)

	sessionDuration := time.Duration(cfg.SessionDurationDays) * 24 * time.Hour

	sd := storydeliv.New(su)
	cd := chdeliv.New(cu)
	td := tagdeliv.New(tu)
	ad := aideliv.New(au)
	authd := authdeliv.New(authu, sessionDuration)

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

	r.Use(middleware.RequestIDMiddleware, middleware.AccessLogMiddleware)

	api := r.PathPrefix("/api").Subrouter()

	api.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}).Methods(http.MethodGet)

	api.HandleFunc("/login", authd.Login).Methods(http.MethodPost)
	api.HandleFunc("/register", authd.Register).Methods(http.MethodPost)
	api.HandleFunc("/logout", authd.Logout).Methods(http.MethodPost)
	api.HandleFunc("/session", authd.GetSession).Methods(http.MethodGet)

	api.HandleFunc("/stories", sd.List).Methods(http.MethodGet)
	api.HandleFunc("/stories/{slug}", sd.GetBySlug).Methods(http.MethodGet)

	api.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Get).Methods(http.MethodGet)

	api.HandleFunc("/tags", td.List).Methods(http.MethodGet)

	api.HandleFunc("/ai/spellcheck", ad.Spellcheck).Methods(http.MethodPost)
	api.HandleFunc("/ai/image-generation", ad.ImageGeneration).Methods(http.MethodPost)
	api.HandleFunc("/ai/jobs/{jobId:"+uuidRe+"}", ad.GetJob).Methods(http.MethodGet)

	protected := api.NewRoute().Subrouter()
	protected.Use(middleware.AuthMiddleware(redisDB))

	protected.HandleFunc("/stories", sd.Create).Methods(http.MethodPost)
	protected.HandleFunc("/stories/{id:"+uuidRe+"}", sd.Patch).Methods(http.MethodPatch)
	protected.HandleFunc("/stories/{id:"+uuidRe+"}", sd.Delete).Methods(http.MethodDelete)
	protected.HandleFunc("/stories/{storyId:"+uuidRe+"}/chapters", cd.CreateUnderStory).Methods(http.MethodPost)
	protected.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Patch).Methods(http.MethodPatch)
	protected.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Delete).Methods(http.MethodDelete)

	return middleware.CORS(r)
}
