package delivery

import (
	"net/http"
	"strings"

	"github.com/fivecode/plotty/core/logger"
	libuc "github.com/fivecode/plotty/core/library/usecase"
	"github.com/fivecode/plotty/core/middleware"
	namederrors "github.com/fivecode/plotty/core/named_errors"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Delivery struct {
	uc *libuc.Usecase
}

func New(uc *libuc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

func (d *Delivery) ListShelf(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	var shelf *string
	if v := strings.TrimSpace(r.URL.Query().Get("shelf")); v != "" {
		shelf = &v
	}
	items, err := d.uc.ListShelf(r.Context(), shelf)
	if err != nil {
		log.Warn().Err(err).Msg("library_delivery: list shelf failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type putShelfBody struct {
	Shelf string `json:"shelf"`
}

func (d *Delivery) PutShelf(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	storyID, err := uuid.Parse(mux.Vars(r)["storyId"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid story id")
		return
	}
	var body putShelfBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := d.uc.SetShelf(r.Context(), storyID, body.Shelf); err != nil {
		log.Warn().Err(err).Msg("library_delivery: put shelf failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *Delivery) DeleteShelf(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	storyID, err := uuid.Parse(mux.Vars(r)["storyId"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid story id")
		return
	}
	if err := d.uc.RemoveShelf(r.Context(), storyID); err != nil {
		log.Warn().Err(err).Msg("library_delivery: delete shelf failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createCollectionBody struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
}

func (d *Delivery) CreateCollection(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	var body createCollectionBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	c, err := d.uc.CreateCollection(r.Context(), body.Title, body.Description)
	if err != nil {
		log.Warn().Err(err).Msg("library_delivery: create collection failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusCreated, map[string]any{"collection": c})
}

func (d *Delivery) ListMyCollections(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utilities.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := d.uc.ListCollectionsForUser(r.Context(), userID)
	if err != nil {
		log.Warn().Err(err).Msg("library_delivery: list collections failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d *Delivery) GetMyCollection(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utilities.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	detail, err := d.uc.GetCollectionDetail(r.Context(), id)
	if err != nil {
		log.Warn().Err(err).Msg("library_delivery: get collection failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	if detail.UserID != userID {
		utilities.WriteError(w, utilities.StatusFromErr(namederrors.ErrNoAccess), namederrors.ErrNoAccess.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"collection": detail})
}

type patchCollectionBody struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

func (d *Delivery) PatchCollection(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body patchCollectionBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Title == nil && body.Description == nil {
		utilities.WriteError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	c, err := d.uc.UpdateCollection(r.Context(), id, body.Title, body.Description)
	if err != nil {
		log.Warn().Err(err).Msg("library_delivery: patch collection failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"collection": c})
}

func (d *Delivery) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := d.uc.DeleteCollection(r.Context(), id); err != nil {
		log.Warn().Err(err).Msg("library_delivery: delete collection failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type addStoryBody struct {
	StoryID uuid.UUID `json:"storyId"`
}

func (d *Delivery) AddStoryToCollection(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	collectionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body addStoryBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := d.uc.AddStoryToCollection(r.Context(), collectionID, body.StoryID); err != nil {
		log.Warn().Err(err).Msg("library_delivery: add story failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *Delivery) RemoveStoryFromCollection(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	collectionID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	storyID, err := uuid.Parse(mux.Vars(r)["storyId"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid story id")
		return
	}
	if err := d.uc.RemoveStoryFromCollection(r.Context(), collectionID, storyID); err != nil {
		log.Warn().Err(err).Msg("library_delivery: remove story failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
