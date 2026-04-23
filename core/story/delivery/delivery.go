package delivery

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/fivecode/plotty/core/logger"
	storyuc "github.com/fivecode/plotty/core/story/usecase"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Delivery struct {
	uc *storyuc.Usecase
}

func New(uc *storyuc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

func parseTagSlugs(r *http.Request) []string {
	var slugs []string
	for _, t := range r.URL.Query()["tag"] {
		t = strings.TrimSpace(t)
		if t != "" {
			slugs = append(slugs, t)
		}
	}
	if csv := strings.TrimSpace(r.URL.Query().Get("tags")); csv != "" {
		for _, p := range strings.Split(csv, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				slugs = append(slugs, p)
			}
		}
	}
	seen := make(map[string]struct{})
	var out []string
	for _, s := range slugs {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func (d *Delivery) List(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page == 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize == 0 {
		pageSize = 20
	}

	items, total, err := d.uc.List(r.Context(), q, parseTagSlugs(r), page, pageSize)
	if err != nil {
		log.Warn().Err(err).Msg("story_delivery: list failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":     page,
			"pageSize": pageSize,
			"total":    total,
		},
	})
}

func (d *Delivery) ListMy(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page == 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize == 0 {
		pageSize = 20
	}

	items, total, err := d.uc.ListMy(r.Context(), q, parseTagSlugs(r), page, pageSize)
	if err != nil {
		log.Warn().Err(err).Msg("story_delivery: list_my failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":     page,
			"pageSize": pageSize,
			"total":    total,
		},
	})
}

type createStoryBody struct {
	Title  string      `json:"title"`
	TagIDs []uuid.UUID `json:"tagIds"`
}

func (d *Delivery) Create(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	var body createStoryBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		log.Warn().Err(err).Msg("story_delivery: create invalid json")
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	s, err := d.uc.Create(r.Context(), body.Title, body.TagIDs)
	if err != nil {
		log.Warn().Err(err).Str("title", body.Title).Msg("story_delivery: create failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	log.Info().Stringer("story_id", s.ID).Msg("story_delivery: created")
	utilities.WriteJSON(w, http.StatusCreated, s)
}

type patchStoryBody struct {
	Title  *string      `json:"title"`
	TagIDs *[]uuid.UUID `json:"tagIds"`
}

func (d *Delivery) Patch(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body patchStoryBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		log.Warn().Err(err).Stringer("story_id", id).Msg("story_delivery: patch invalid json")
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	s, err := d.uc.Update(r.Context(), id, body.Title, body.TagIDs)
	if err != nil {
		log.Warn().Err(err).Stringer("story_id", id).Msg("story_delivery: patch failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, s)
}

func (d *Delivery) GetBySlug(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	slug := strings.TrimSpace(mux.Vars(r)["slug"])
	if slug == "" {
		utilities.WriteError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	detail, err := d.uc.GetBySlug(r.Context(), slug)
	if err != nil {
		log.Warn().Err(err).Str("slug", slug).Msg("story_delivery: get by slug failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, detail)
}

func (d *Delivery) Delete(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := d.uc.Delete(r.Context(), id); err != nil {
		log.Warn().Err(err).Stringer("story_id", id).Msg("story_delivery: delete failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	log.Info().Stringer("story_id", id).Msg("story_delivery: deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (d *Delivery) GetSimilar(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	items, err := d.uc.GetSimilar(r.Context(), id)
	if err != nil {
		log.Warn().Err(err).Msg("story_delivery: similar failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (d *Delivery) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	analytics, err := d.uc.GetAnalytics(r.Context(), id)
	if err != nil {
		log.Warn().Err(err).Msg("story_delivery: analytics failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"analytics": analytics,
	})
}
