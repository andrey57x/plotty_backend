package delivery

import (
	"net/http"
	"strings"

	"github.com/fivecode/plotty/core/logger"
	taguc "github.com/fivecode/plotty/core/tag/usecase"
	"github.com/fivecode/plotty/core/utilities"
)

type Delivery struct {
	uc *taguc.Usecase
}

func New(uc *taguc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

func (d *Delivery) List(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	category := strings.TrimSpace(r.URL.Query().Get("category"))
	items, err := d.uc.List(r.Context(), category)
	if err != nil {
		log.Warn().Err(err).Str("category", category).Msg("tag_delivery: list failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
