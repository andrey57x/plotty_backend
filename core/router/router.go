package router

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	aideliv "github.com/fivecode/plotty/core/ai/delivery"
	airepo "github.com/fivecode/plotty/core/ai/repository"
	aiuc "github.com/fivecode/plotty/core/ai/usecase"
	authdeliv "github.com/fivecode/plotty/core/auth/delivery"
	authrepo "github.com/fivecode/plotty/core/auth/repository"
	authuc "github.com/fivecode/plotty/core/auth/usecase"
	creddeliv "github.com/fivecode/plotty/core/credits/delivery"
	credrepo "github.com/fivecode/plotty/core/credits/repository"
	creduc "github.com/fivecode/plotty/core/credits/usecase"
	chdeliv "github.com/fivecode/plotty/core/chapter/delivery"
	chrepo "github.com/fivecode/plotty/core/chapter/repository"
	chuc "github.com/fivecode/plotty/core/chapter/usecase"
	commentdeliv "github.com/fivecode/plotty/core/comment/delivery"
	commentrepo "github.com/fivecode/plotty/core/comment/repository"
	commentuc "github.com/fivecode/plotty/core/comment/usecase"
	"github.com/fivecode/plotty/core/config"
	libdeliv "github.com/fivecode/plotty/core/library/delivery"
	librepo "github.com/fivecode/plotty/core/library/repository"
	libuc "github.com/fivecode/plotty/core/library/usecase"
	likedeliv "github.com/fivecode/plotty/core/like/delivery"
	likerepo "github.com/fivecode/plotty/core/like/repository"
	likeuc "github.com/fivecode/plotty/core/like/usecase"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/ml"
	"github.com/fivecode/plotty/core/profile"
	"github.com/fivecode/plotty/core/redis"
	storydeliv "github.com/fivecode/plotty/core/story/delivery"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	storyuc "github.com/fivecode/plotty/core/story/usecase"
	tagdeliv "github.com/fivecode/plotty/core/tag/delivery"
	tagrepo "github.com/fivecode/plotty/core/tag/repository"
	taguc "github.com/fivecode/plotty/core/tag/usecase"
	storage "github.com/fivecode/plotty/internal/infrastructure/minio"
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
	lr := likerepo.New(pool)
	comr := commentrepo.New(pool)
	credr := credrepo.New(pool)

	tu := taguc.New(tr)
	mlClient := ml.NewClient(cfg.MLBaseURL)

	su := storyuc.New(sr, tr, cr, mlClient)
	cu := chuc.New(cr, sr, rmqChan, mlClient)
	cu.SetAuthorChecker(su)

	credu := creduc.New(credr, cfg.YooMoneyWallet, cfg.YooMoneySecret, cfg.FrontendURL)
	au := aiuc.New(ar, cr, sr, rmqChan, credr)
	authu := authuc.New(authr)
	lu := likeuc.New(lr)
	comu := commentuc.New(comr)

	libr := librepo.New(pool)
	libu := libuc.New(libr, sr)

	sessionDuration := time.Duration(cfg.SessionDurationDays) * 24 * time.Hour

	var st *storage.MinioStorage
	if strings.TrimSpace(cfg.MinioEndpoint) != "" && strings.TrimSpace(cfg.MinioBucket) != "" {
		if ss, err := storage.NewMinioStorage(cfg.MinioEndpoint, cfg.MinioUser, cfg.MinioPassword, cfg.MinioBucket, cfg.MinioPublicURL); err == nil {
			st = ss
		}
	}

	sd := storydeliv.New(su)
	cd := chdeliv.New(cu)
	td := tagdeliv.New(tu)
	ad := aideliv.New(au)
	authd := authdeliv.New(authu, sessionDuration, st)
	ld := likedeliv.New(lu)
	comd := commentdeliv.New(comu)
	libd := libdeliv.New(libu)
	prof := profile.New(authu, su, libu)
	credd := creddeliv.New(credu)

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
	api.Use(middleware.OptionalAuthMiddleware(redisDB))

	api.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}).Methods(http.MethodGet)

	api.HandleFunc("/login", authd.Login).Methods(http.MethodPost)
	api.HandleFunc("/register", authd.Register).Methods(http.MethodPost)
	api.HandleFunc("/logout", authd.Logout).Methods(http.MethodPost)
	api.HandleFunc("/session", authd.GetSession).Methods(http.MethodGet)

	const usernameRe = `[a-zA-Z0-9_]{3,40}`
	api.HandleFunc("/users/{username:"+usernameRe+"}", prof.GetPublicProfile).Methods(http.MethodGet)
	api.HandleFunc("/users/{username:"+usernameRe+"}/stories", prof.GetPublicStories).Methods(http.MethodGet)
	api.HandleFunc("/users/{username:"+usernameRe+"}/collections", prof.GetPublicCollections).Methods(http.MethodGet)
	api.HandleFunc("/users/{username:"+usernameRe+"}/collections/{collectionId:"+uuidRe+"}", prof.GetPublicCollection).Methods(http.MethodGet)

	api.HandleFunc("/stories", sd.List).Methods(http.MethodGet)
	api.HandleFunc("/stories/{id:"+uuidRe+"}/similar", sd.GetSimilar).Methods(http.MethodGet)

	protected := api.NewRoute().Subrouter()
	protected.Use(middleware.AuthMiddleware(redisDB))

	protected.HandleFunc("/stories/mine", sd.ListMy).Methods(http.MethodGet)
	protected.HandleFunc("/stories", sd.Create).Methods(http.MethodPost)
	protected.HandleFunc("/stories/{id:"+uuidRe+"}", sd.Patch).Methods(http.MethodPatch)
	protected.HandleFunc("/stories/{id:"+uuidRe+"}", sd.Delete).Methods(http.MethodDelete)
	protected.HandleFunc("/stories/{id:"+uuidRe+"}/like", ld.Like).Methods(http.MethodPost)
	protected.HandleFunc("/stories/{id:"+uuidRe+"}/like", ld.Unlike).Methods(http.MethodDelete)
	protected.HandleFunc("/stories/{id:"+uuidRe+"}/analytics", sd.GetAnalytics).Methods(http.MethodGet)

	protected.HandleFunc("/stories/{storyId:"+uuidRe+"}/chapters", cd.CreateUnderStory).Methods(http.MethodPost)
	protected.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Patch).Methods(http.MethodPatch)
	protected.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Delete).Methods(http.MethodDelete)
	protected.HandleFunc("/chapters/{id:"+uuidRe+"}/publish", cd.Publish).Methods(http.MethodPost)
	protected.HandleFunc("/chapters/{id:"+uuidRe+"}/discard-draft", cd.DiscardDraft).Methods(http.MethodPost)
	protected.HandleFunc("/chapters/{id:"+uuidRe+"}/canon-check", ad.CanonCheck).Methods(http.MethodPost)
	protected.HandleFunc("/chapters/{id:"+uuidRe+"}/comments", comd.Create).Methods(http.MethodPost)
	protected.HandleFunc("/comments/{commentId:"+uuidRe+"}", comd.Delete).Methods(http.MethodDelete)

	protected.HandleFunc("/ai/spellcheck", ad.Spellcheck).Methods(http.MethodPost)
	protected.HandleFunc("/ai/image-generation", ad.ImageGeneration).Methods(http.MethodPost)
	protected.HandleFunc("/ai/logic-check", ad.LogicCheck).Methods(http.MethodPost)
	protected.HandleFunc("/ai/jobs/{jobId:"+uuidRe+"}", ad.GetJob).Methods(http.MethodGet)

	protected.HandleFunc("/credits/balance", credd.GetBalance).Methods(http.MethodGet)
	protected.HandleFunc("/credits/transactions", credd.GetTransactions).Methods(http.MethodGet)
	protected.HandleFunc("/credits/packages", credd.GetPackages).Methods(http.MethodGet)
	protected.HandleFunc("/credits/purchase", credd.InitiatePurchase).Methods(http.MethodPost)

	api.HandleFunc("/webhooks/yoomoney", credd.HandleIPN).Methods(http.MethodPost)

	protected.HandleFunc("/profile", authd.UpdateProfile).Methods(http.MethodPatch)
	protected.HandleFunc("/profile/avatar", authd.UploadAvatar).Methods(http.MethodPost)

	protected.HandleFunc("/me/library/shelf", libd.ListShelf).Methods(http.MethodGet)
	protected.HandleFunc("/me/library/shelf/{storyId:"+uuidRe+"}", libd.PutShelf).Methods(http.MethodPut)
	protected.HandleFunc("/me/library/shelf/{storyId:"+uuidRe+"}", libd.DeleteShelf).Methods(http.MethodDelete)

	protected.HandleFunc("/me/collections", libd.ListMyCollections).Methods(http.MethodGet)
	protected.HandleFunc("/me/collections", libd.CreateCollection).Methods(http.MethodPost)
	protected.HandleFunc("/me/collections/{id:"+uuidRe+"}", libd.GetMyCollection).Methods(http.MethodGet)
	protected.HandleFunc("/me/collections/{id:"+uuidRe+"}", libd.PatchCollection).Methods(http.MethodPatch)
	protected.HandleFunc("/me/collections/{id:"+uuidRe+"}", libd.DeleteCollection).Methods(http.MethodDelete)
	protected.HandleFunc("/me/collections/{id:"+uuidRe+"}/stories", libd.AddStoryToCollection).Methods(http.MethodPost)
	protected.HandleFunc("/me/collections/{id:"+uuidRe+"}/stories/{storyId:"+uuidRe+"}", libd.RemoveStoryFromCollection).Methods(http.MethodDelete)

	api.HandleFunc("/stories/{slug}", sd.GetBySlug).Methods(http.MethodGet)
	api.HandleFunc("/stories/{slug}/chapters/viewed", sd.GetChaptersViewed).Methods(http.MethodGet)
	api.HandleFunc("/chapters/{id:"+uuidRe+"}/view", cd.AddView).Methods(http.MethodPost)
	api.HandleFunc("/chapters/{id:"+uuidRe+"}/viewed", cd.IsViewed).Methods(http.MethodGet)

	api.HandleFunc("/chapters/{id:"+uuidRe+"}", cd.Get).Methods(http.MethodGet)
	api.HandleFunc("/chapters/{id:"+uuidRe+"}/wiki", cd.GetWiki).Methods(http.MethodGet)
	api.HandleFunc("/chapters/{id:"+uuidRe+"}/comments", comd.List).Methods(http.MethodGet)

	api.HandleFunc("/tags", td.List).Methods(http.MethodGet)

	return middleware.CORS(r)
}
