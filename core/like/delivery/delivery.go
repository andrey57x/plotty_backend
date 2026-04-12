package delivery

import (
	"net/http"

	likeuc "github.com/fivecode/plotty/core/like/usecase"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Delivery struct {
	uc *likeuc.Usecase
}

func New(uc *likeuc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

func (d *Delivery) Like(w http.ResponseWriter, r *http.Request) {
	storyID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid story id")
		return
	}
	if err := d.uc.Like(r.Context(), storyID); err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	count, liked, err := d.uc.Status(r.Context(), storyID)
	if err != nil {
		utilities.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"likesCount": count,
		"likedByMe":  liked,
	})
}

func (d *Delivery) Unlike(w http.ResponseWriter, r *http.Request) {
	storyID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid story id")
		return
	}
	if err := d.uc.Unlike(r.Context(), storyID); err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	count, liked, err := d.uc.Status(r.Context(), storyID)
	if err != nil {
		utilities.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"likesCount": count,
		"likedByMe":  liked,
	})
}
