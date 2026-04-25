package profile

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/fivecode/plotty/core/logger"
	libuc "github.com/fivecode/plotty/core/library/usecase"
	"github.com/fivecode/plotty/core/models"
	storyuc "github.com/fivecode/plotty/core/story/usecase"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type authPublic interface {
	GetPublicProfileByUsername(ctx context.Context, username string) (*models.PublicUserProfile, error)
}

type Handler struct {
	Auth    authPublic
	Story   *storyuc.Usecase
	Library *libuc.Usecase
}

func New(auth authPublic, su *storyuc.Usecase, lu *libuc.Usecase) *Handler {
	return &Handler{Auth: auth, Story: su, Library: lu}
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

func (h *Handler) GetPublicProfile(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	username := strings.TrimSpace(mux.Vars(r)["username"])
	if username == "" {
		utilities.WriteError(w, http.StatusBadRequest, "invalid username")
		return
	}
	p, err := h.Auth.GetPublicProfileByUsername(r.Context(), username)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Msg("profile: get public user failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"profile": p})
}

func (h *Handler) GetPublicStories(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	username := strings.TrimSpace(mux.Vars(r)["username"])
	if username == "" {
		utilities.WriteError(w, http.StatusBadRequest, "invalid username")
		return
	}
	p, err := h.Auth.GetPublicProfileByUsername(r.Context(), username)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Msg("profile: get public user failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page == 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize == 0 {
		pageSize = 20
	}

	items, total, err := h.Story.ListPublishedByAuthor(r.Context(), p.ID, q, parseTagSlugs(r), page, pageSize)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Msg("profile: list author stories failed")
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

func (h *Handler) GetPublicCollections(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	username := strings.TrimSpace(mux.Vars(r)["username"])
	if username == "" {
		utilities.WriteError(w, http.StatusBadRequest, "invalid username")
		return
	}
	p, err := h.Auth.GetPublicProfileByUsername(r.Context(), username)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Msg("profile: get public user failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	items, err := h.Library.ListCollectionsForUser(r.Context(), p.ID)
	if err != nil {
		log.Warn().Err(err).Msg("profile: list public collections failed")
		utilities.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) GetPublicCollection(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	username := strings.TrimSpace(mux.Vars(r)["username"])
	collectionID, err := uuid.Parse(mux.Vars(r)["collectionId"])
	if err != nil || username == "" {
		utilities.WriteError(w, http.StatusBadRequest, "invalid request")
		return
	}
	owner, err := h.Auth.GetPublicProfileByUsername(r.Context(), username)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Msg("profile: get public user failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	detail, err := h.Library.GetCollectionDetail(r.Context(), collectionID)
	if err != nil {
		log.Warn().Err(err).Msg("profile: get public collection failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	if detail.UserID != owner.ID {
		utilities.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	utilities.WriteJSON(w, http.StatusOK, map[string]any{"collection": detail})
}
