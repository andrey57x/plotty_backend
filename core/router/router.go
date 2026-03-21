package router

import (
	"net/http"
	"strings"

	aideliv "github.com/fivecode/plotty/core/ai/delivery"
	airepo "github.com/fivecode/plotty/core/ai/repository"
	aiuc "github.com/fivecode/plotty/core/ai/usecase"
	chdeliv "github.com/fivecode/plotty/core/chapter/delivery"
	chrepo "github.com/fivecode/plotty/core/chapter/repository"
	chuc "github.com/fivecode/plotty/core/chapter/usecase"
	"github.com/fivecode/plotty/core/ml"
	storydeliv "github.com/fivecode/plotty/core/story/delivery"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	storyuc "github.com/fivecode/plotty/core/story/usecase"
	tagdeliv "github.com/fivecode/plotty/core/tag/delivery"
	tagrepo "github.com/fivecode/plotty/core/tag/repository"
	taguc "github.com/fivecode/plotty/core/tag/usecase"
	"github.com/fivecode/plotty/internal/config"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uuidRe = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

func NewRouter(cfg *config.Config, pool *pgxpool.Pool) http.Handler {
	tr := tagrepo.New(pool)
	sr := storyrepo.New(pool)
	cr := chrepo.New(pool)
	ar := airepo.New(pool)

	tu := taguc.New(tr)
	su := storyuc.New(sr, tr, cr)
	cu := chuc.New(cr, sr)

	mlc := ml.NewClient(strings.TrimRight(cfg.MLBaseURL, "/"))
	au := aiuc.New(ar, cr, mlc)

	sd := storydeliv.New(su)
	cd := chdeliv.New(cu)
	td := tagdeliv.New(tu)
	ad := aideliv.New(au)

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

	return r
}
