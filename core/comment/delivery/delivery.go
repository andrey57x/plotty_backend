package delivery

import (
	"net/http"
	"strconv"

	commentuc "github.com/fivecode/plotty/core/comment/usecase"
	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Delivery struct {
	uc *commentuc.Usecase
}

func New(uc *commentuc.Usecase) *Delivery {
	return &Delivery{uc: uc}
}

type createCommentBody struct {
	Content string `json:"content"`
}

func (d *Delivery) Create(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	chapterID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid chapter id")
		return
	}
	var body createCommentBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		log.Warn().Err(err).Stringer("chapter_id", chapterID).Msg("comment_delivery: create invalid json")
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	comment, err := d.uc.Create(r.Context(), chapterID, body.Content)
	if err != nil {
		log.Warn().Err(err).Stringer("chapter_id", chapterID).Msg("comment_delivery: create failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	log.Info().Stringer("comment_id", comment.ID).Msg("comment_delivery: created")
	utilities.WriteJSON(w, http.StatusCreated, comment)
}

func (d *Delivery) List(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	chapterID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid chapter id")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page == 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize == 0 {
		pageSize = 20
	}
	comments, total, err := d.uc.List(r.Context(), chapterID, page, pageSize)
	if err != nil {
		log.Warn().Err(err).Stringer("chapter_id", chapterID).Msg("comment_delivery: list failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	utilities.WriteJSON(w, http.StatusOK, map[string]any{
		"items": comments,
		"pagination": map[string]any{
			"page":     page,
			"pageSize": pageSize,
			"total":    total,
		},
	})
}

func (d *Delivery) Update(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	commentID, err := uuid.Parse(mux.Vars(r)["commentId"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid comment id")
		return
	}
	var body createCommentBody
	if err := utilities.DecodeJSON(r, &body); err != nil {
		log.Warn().Err(err).Stringer("comment_id", commentID).Msg("comment_delivery: update invalid json")
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	comment, err := d.uc.Update(r.Context(), commentID, body.Content)
	if err != nil {
		log.Warn().Err(err).Stringer("comment_id", commentID).Msg("comment_delivery: update failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	log.Info().Stringer("comment_id", commentID).Msg("comment_delivery: updated")
	utilities.WriteJSON(w, http.StatusOK, comment)
}

func (d *Delivery) Delete(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	commentID, err := uuid.Parse(mux.Vars(r)["commentId"])
	if err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid comment id")
		return
	}
	if err := d.uc.Delete(r.Context(), commentID); err != nil {
		log.Warn().Err(err).Stringer("comment_id", commentID).Msg("comment_delivery: delete failed")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}
	log.Info().Stringer("comment_id", commentID).Msg("comment_delivery: deleted")
	w.WriteHeader(http.StatusNoContent)
}
