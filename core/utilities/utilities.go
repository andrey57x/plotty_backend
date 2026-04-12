package utilities

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/rs/zerolog/log"
)

func init() {
	govalidator.SetFieldsRequiredByDefault(false)

	govalidator.CustomTypeTagMap.Set("password", govalidator.CustomTypeValidator(func(i interface{}, o interface{}) bool {
		s, ok := i.(string)
		if !ok {
			return false
		}
		return len(s) >= 8
	}))
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationErrors struct {
	Errors []FieldError `json:"errors"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("json encode error")
	}
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, ErrorResponse{Error: message})
}

func WriteValidationErrors(w http.ResponseWriter, code int, errs []FieldError) {
	WriteJSON(w, code, ValidationErrors{Errors: errs})
}

func WriteValidationError(w http.ResponseWriter, code int, err error) {
	if ge, ok := err.(govalidator.Errors); ok {
		out := make([]FieldError, 0, len(ge))
		for _, e := range ge.Errors() {
			field, msg := parseGovalidatorError(e.Error())
			out = append(out, FieldError{
				Field:   field,
				Message: msg,
			})
		}
		WriteValidationErrors(w, code, out)
		return
	}

	WriteError(w, code, err.Error())
}

func parseGovalidatorError(s string) (field, message string) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(s)
}

func ValidateStruct(s interface{}) error {
	_, err := govalidator.ValidateStruct(s)
	return err
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
	case errors.Is(err, named_errors.ErrNoAccess):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
