package delivery

import (
	"net/http"

	chapteruc "github.com/fivecode/plotty/core/chapter/usecase"
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
	storyID, err := uuid.Parse(mux.Vars(r)["storyId"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid storyId")
		return
	}
	var body createChapterBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ch, err := d.uc.Create(r.Context(), storyID, body.Title, body.Content)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusCreated, ch)
}

type patchChapterBody struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

func (d *Delivery) Patch(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body patchChapterBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	ch, err := d.uc.Update(r.Context(), id, body.Title, body.Content)
	if err != nil {
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
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ch, err := d.uc.Get(r.Context(), id)
	if err != nil {
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

func (d *Delivery) Publish(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid chapter id")
		return
	}

	// Опционально: здесь должна быть проверка на то, что текущий юзер — автор истории. 
	// Но пока просто вызываем UseCase.

	if err := d.uc.Publish(r.Context(), id); err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	utilities.WriteJSON(w, http.StatusOK, map[string]string{"status": "published"})
}
