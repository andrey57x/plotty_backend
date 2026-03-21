package delivery

import (
	"net/http"

	aiuc "github.com/fivecode/plotty/core/ai/usecase"
	"github.com/fivecode/plotty/core/constants"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Delivery struct {
	uc *aiuc.Usecase
}

func New(uc *aiuc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

type spellcheckBody struct {
	ChapterID uuid.UUID `json:"chapterId"`
	Content   string    `json:"content"`
}

func (d *Delivery) Spellcheck(w http.ResponseWriter, r *http.Request) {
	var body spellcheckBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	jobID, err := d.uc.StartSpellcheck(r.Context(), body.ChapterID, body.Content)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"jobId":  jobID.String(),
		"status": constants.AIJobStatusProcessing,
	})
}

type imageGenBody struct {
	ChapterID uuid.UUID `json:"chapterId"`
	Content   string    `json:"content"`
	Prompt    string    `json:"prompt"`
}

func (d *Delivery) ImageGeneration(w http.ResponseWriter, r *http.Request) {
	var body imageGenBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	jobID, err := d.uc.StartImageGeneration(r.Context(), body.ChapterID, body.Content, body.Prompt)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"jobId":  jobID.String(),
		"status": constants.AIJobStatusProcessing,
	})
}

func (d *Delivery) GetJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["jobId"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid jobId")
		return
	}
	out, err := d.uc.GetJobView(r.Context(), id)
	if err != nil {
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, out)
}
