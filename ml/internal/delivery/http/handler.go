package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/google/uuid"
)

type EmbeddingsProvider interface {
	GetEmbedding(ctx context.Context, text string) ([]float32, error)
}

type Handler struct {
	repo       repository.MLRepository
	embeddings EmbeddingsProvider
}

func NewHandler(repo repository.MLRepository, embeddings EmbeddingsProvider) *http.ServeMux {
	h := &Handler{repo: repo, embeddings: embeddings}
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/wiki", h.GetWiki)
	mux.HandleFunc("/internal/similar", h.GetSimilar)
	mux.HandleFunc("/internal/search_semantic", h.SearchSemantic)
	return mux
}

func (h *Handler) GetWiki(w http.ResponseWriter, r *http.Request) {
	chapterIDStr := r.URL.Query().Get("chapter_id")
	if chapterIDStr == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
		return
	}

	chapterID, err := uuid.Parse(chapterIDStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
		return
	}

	loreStr, err := h.repo.GetLoreByChapterID(r.Context(), chapterID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if loreStr == "" {
		loreStr = "{}"
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(loreStr))
}

func (h *Handler) GetSimilar(w http.ResponseWriter, r *http.Request) {
	storyIDStr := r.URL.Query().Get("story_id")
	if storyIDStr == "" {
		w.Write([]byte("[]"))
		return
	}
	storyID, err := uuid.Parse(storyIDStr)
	if err != nil {
		w.Write([]byte("[]"))
		return
	}

	ids, err := h.repo.GetSimilarStories(r.Context(), storyID, 5)
	if err != nil || len(ids) == 0 {
		w.Write([]byte("[]"))
		return
	}

	out, _ := json.Marshal(ids)
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func (h *Handler) SearchSemantic(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if strings.TrimSpace(q) == "" {
		w.Write([]byte("[]"))
		return
	}

	emb, err := h.embeddings.GetEmbedding(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ids, err := h.repo.SearchStoriesByEmbedding(r.Context(), emb, 100)
	if err != nil || len(ids) == 0 {
		w.Write([]byte("[]"))
		return
	}

	out, _ := json.Marshal(ids)
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}
