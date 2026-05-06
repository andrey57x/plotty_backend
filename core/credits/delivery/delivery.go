package delivery

import (
	"net/http"

	creduc "github.com/fivecode/plotty/core/credits/usecase"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/utilities"
)

type Delivery struct {
	uc *creduc.Usecase
}

func New(uc *creduc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

func (d *Delivery) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utilities.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	balance, err := d.uc.GetBalance(r.Context(), userID)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"balance": balance})
}

func (d *Delivery) GetTransactions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utilities.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	txs, err := d.uc.GetTransactions(r.Context(), userID)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	if txs == nil {
		utilities.WriteJSON(w, http.StatusOK, []any{})
		return
	}
	utilities.WriteJSON(w, http.StatusOK, txs)
}

func (d *Delivery) GetPackages(w http.ResponseWriter, r *http.Request) {
	utilities.WriteJSON(w, http.StatusOK, d.uc.GetPackages())
}

func (d *Delivery) InitiatePurchase(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utilities.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		PackageID int `json:"packageId"`
	}
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	payURL, err := d.uc.InitiatePurchase(userID, body.PackageID)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]string{"payUrl": payURL})
}

func (d *Delivery) HandleIPN(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := d.uc.HandleIPN(
		r.Context(),
		r.FormValue("notification_type"),
		r.FormValue("operation_id"),
		r.FormValue("amount"),
		r.FormValue("currency"),
		r.FormValue("datetime"),
		r.FormValue("sender"),
		r.FormValue("codepro"),
		r.FormValue("label"),
		r.FormValue("sha1_hash"),
	)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}
