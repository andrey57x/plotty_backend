package delivery

import (
	"net/http"

	chapteruc "github.com/fivecode/plotty/core/chapter/usecase"
	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Delivery struct {
	uc *chapteruc.Usecase
}

func New(uc *chapteruc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

type createChapterBody struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (d *Delivery) CreateUnderStory(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	storyID, err := uuid.Parse(mux.Vars(r)["storyId"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid storyId")
		return
	}
	var body createChapterBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		log.Warn().Err(err).Stringer("story_id", storyID).Msg("chapter_delivery: create invalid json")
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ch, err := d.uc.Create(r.Context(), storyID, body.Title, body.Content)
	if err != nil {
		log.Warn().Err(err).Stringer("story_id", storyID).Msg("chapter_delivery: create failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	log.Info().Stringer("chapter_id", ch.ID).Msg("chapter_delivery: created")
	utilities.WriteJSON(w, http.StatusCreated, ch)
}

type patchChapterBody struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

func (d *Delivery) Patch(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body patchChapterBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		log.Warn().Err(err).Stringer("chapter_id", id).Msg("chapter_delivery: patch invalid json")
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ch, err := d.uc.Update(r.Context(), id, body.Title, body.Content)
	if err != nil {
		log.Warn().Err(err).Stringer("chapter_id", id).Msg("chapter_delivery: patch failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, ch)
}

type chapterGetResponse struct {
	ID        uuid.UUID `json:"id"`
	StoryID   uuid.UUID `json:"storyId"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	UpdatedAt string    `json:"updatedAt"`
	ImageURL  *string   `json:"imageUrl,omitempty"`
}

func (d *Delivery) Get(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ch, err := d.uc.Get(r.Context(), id)
	if err != nil {
		log.Warn().Err(err).Stringer("chapter_id", id).Msg("chapter_delivery: get failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, chapterGetResponse{
		ID:        ch.ID,
		StoryID:   ch.StoryID,
		Title:     ch.Title,
		Content:   ch.Content,
		UpdatedAt: ch.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		ImageURL:  ch.ImageURL,
	})
}

func (d *Delivery) Delete(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := d.uc.Delete(r.Context(), id); err != nil {
		log.Warn().Err(err).Stringer("chapter_id", id).Msg("chapter_delivery: delete failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	log.Info().Stringer("chapter_id", id).Msg("chapter_delivery: deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (d *Delivery) Publish(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid chapter id")
		return
	}
	if err := d.uc.Publish(r.Context(), id); err != nil {
		log.Warn().Err(err).Stringer("chapter_id", id).Msg("chapter_delivery: publish failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	log.Info().Stringer("chapter_id", id).Msg("chapter_delivery: published")
	utilities.WriteJSON(w, http.StatusOK, map[string]string{"status": "published"})
}

func (d *Delivery) GetWiki(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	wikiJSON, err := d.uc.GetWiki(r.Context(), id)
	if err != nil {
		log.Warn().Err(err).Stringer("chapter_id", id).Msg("chapter_delivery: get wiki failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(wikiJSON)
}

func (d *Delivery) AddView(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	
	_ = d.uc.AddView(r.Context(), id)
	w.WriteHeader(http.StatusOK)
}
