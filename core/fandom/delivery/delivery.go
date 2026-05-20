package delivery

import (
	"net/http"

	fandomuc "github.com/fivecode/plotty/core/fandom/usecase"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Delivery struct {
	uc *fandomuc.Usecase
}

func New(uc *fandomuc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

type suggestRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (d *Delivery) Suggest(w http.ResponseWriter, r *http.Request) {
	var body suggestRequest
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	f, err := d.uc.Suggest(r.Context(), body.Name, body.Description)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	utilities.WriteJSON(w, http.StatusCreated, f)
}

func (d *Delivery) ListPending(w http.ResponseWriter, r *http.Request) {
	// Для простоты не парсим page/pageSize, отдаем первые 50
	items, err := d.uc.ListPending(r.Context(), 1, 50)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (d *Delivery) Approve(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := d.uc.Approve(r.Context(), id); err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (d *Delivery) Reject(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := d.uc.Reject(r.Context(), id); err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}
