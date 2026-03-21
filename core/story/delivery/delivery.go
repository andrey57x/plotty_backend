package delivery

import (
	"net/http"
	"strconv"
	"strings"

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
	var body createStoryBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	s, err := d.uc.Create(r.Context(), body.Title, body.TagIDs)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusCreated, s)
}

type patchStoryBody struct {
	Title  *string      `json:"title"`
	TagIDs *[]uuid.UUID `json:"tagIds"`
}

func (d *Delivery) Patch(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body patchStoryBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	s, err := d.uc.Update(r.Context(), id, body.Title, body.TagIDs)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, s)
}

func (d *Delivery) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(mux.Vars(r)["slug"])
	if slug == "" {
		utilities.WriteError(w, http.StatusBadRequest, "invalid slug")
		return
	}
	detail, err := d.uc.GetBySlug(r.Context(), slug)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, detail)
}

func (d *Delivery) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := d.uc.Delete(r.Context(), id); err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
