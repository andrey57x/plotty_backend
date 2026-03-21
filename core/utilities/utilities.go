package utilities

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/fivecode/plotty/core/named_errors"
)

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

func DecodeJSON(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func StatusFromErr(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, named_errors.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, named_errors.ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, named_errors.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
