package named_errors

import "errors"

var (
	ErrNotFound               = errors.New("not found")
	ErrConflict               = errors.New("conflict")
	ErrInvalidInput           = errors.New("invalid input")
	ErrUserExists             = errors.New("user already exists")
	ErrInvalidEmailOrPassword = errors.New("invalid email or password")
	ErrInvalidSession         = errors.New("invalid session")
	ErrNoAccess               = errors.New("no access")
	ErrInsufficientCredits    = errors.New("insufficient credits")
)
