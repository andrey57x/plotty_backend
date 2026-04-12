package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/models"
	namederrors "github.com/fivecode/plotty/core/named_errors"
	"github.com/fivecode/plotty/core/utilities"
	"github.com/pkg/errors"
)

type AuthDelivery struct {
	SessionDuration time.Duration
	Usecase         AuthUsecase
}

type AuthUsecase interface {
	Login(ctx context.Context, email string, password string) (*models.User, string, error)
	Register(ctx context.Context, email string, password string) (*models.User, string, error)
	Logout(ctx context.Context, sessionID string) error
	GetUserBySession(ctx context.Context, sessionID string) (*models.User, error)
	UpdateProfile(ctx context.Context, userID uint64, username *string, avatarURL *string) (*models.User, error)
}

func New(uc AuthUsecase, sessionDuration time.Duration) *AuthDelivery {
	return &AuthDelivery{
		SessionDuration: sessionDuration,
		Usecase:         uc,
	}
}

type loginRequest struct {
	Email    string `json:"email" valid:"required,email"`
	Password string `json:"password" valid:"required,password"`
}

type registerRequest struct {
	Email           string `json:"email" valid:"required,email"`
	Password        string `json:"password" valid:"required,password"`
	ConfirmPassword string `json:"confirm_password" valid:"required,password"`
}

func (d *AuthDelivery) Login(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Error().Err(err).Msg("failed to close request body")
		}
	}()

	var req loginRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Warn().Err(err).Msg("invalid json body")
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := utilities.ValidateStruct(req); err != nil {
		log.Warn().Err(err).Msg("validation failed")
		utilities.WriteValidationError(w, http.StatusBadRequest, err)
		return
	}

	user, sessionID, err := d.Usecase.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		log.Warn().Err(err).Str("email", req.Email).Msg("login failed")
		utilities.WriteError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	expiration := time.Now().Add(d.SessionDuration)
	session := &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		Expires:  expiration,
		HttpOnly: true,
	}
	http.SetCookie(w, session)

	log.Info().Uint64("user_id", user.ID).Msg("user logged in successfully")
	utilities.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

func (d *AuthDelivery) Register(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())

	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Error().Err(err).Msg("failed to close request body")
		}
	}()

	var req registerRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Warn().Err(err).Msg("invalid json body for registration")
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if err := utilities.ValidateStruct(req); err != nil {
		log.Warn().Err(err).Msg("validation failed")
		utilities.WriteValidationError(w, http.StatusBadRequest, err)
		return
	}

	if req.Password != req.ConfirmPassword {
		log.Warn().Msg("passwords do not match")
		utilities.WriteError(w, http.StatusBadRequest, "passwords do not match")
		return
	}

	user, sessionID, err := d.Usecase.Register(r.Context(), req.Email, req.Password)
	if errors.Is(err, namederrors.ErrUserExists) {
		log.Warn().Str("email", req.Email).Msg("user already exists")
		utilities.WriteError(w, http.StatusBadRequest, "user already exists")
		return
	}
	if err != nil {
		log.Error().Err(err).Str("email", req.Email).Msg("registration failed")
		utilities.WriteError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	expiration := time.Now().Add(d.SessionDuration)
	session := &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		Expires:  expiration,
		HttpOnly: true,
	}
	http.SetCookie(w, session)

	log.Info().Uint64("user_id", user.ID).Msg("user registered successfully")
	utilities.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"user": user,
	})
}

func (d *AuthDelivery) Logout(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	session, err := r.Cookie("session_id")
	if errors.Is(err, http.ErrNoCookie) {
		log.Info().Msg("no session cookie found for logout")
		utilities.WriteError(w, http.StatusBadRequest, "no session cookie")
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("error getting session cookie")
		utilities.WriteError(w, http.StatusInternalServerError, "failed to get session cookie")
		return
	}

	err = d.Usecase.Logout(r.Context(), session.Value)
	if errors.Is(err, namederrors.ErrInvalidSession) {
		log.Warn().Msg("logout with invalid session")
		utilities.WriteError(w, http.StatusBadRequest, "invalid session")
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("failed to logout")
		utilities.WriteError(w, http.StatusInternalServerError, "failed to logout")
		return
	}

	session.Expires = time.Now().Add(-1 * time.Hour)
	http.SetCookie(w, session)

	log.Info().Msg("user logged out successfully")
	utilities.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (d *AuthDelivery) GetSession(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	session, err := r.Cookie("session_id")
	if errors.Is(err, http.ErrNoCookie) {
		log.Info().Msg("no session cookie found")
		utilities.WriteError(w, http.StatusUnauthorized, "no session")
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("error getting session cookie")
		utilities.WriteError(w, http.StatusInternalServerError, "failed to get session cookie")
		return
	}

	user, err := d.Usecase.GetUserBySession(r.Context(), session.Value)
	if errors.Is(err, namederrors.ErrInvalidSession) {
		log.Warn().Msg("invalid session")
		utilities.WriteError(w, http.StatusUnauthorized, "invalid session")
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("failed to get user by session")
		utilities.WriteError(w, http.StatusInternalServerError, "failed to get session")
		return
	}

	utilities.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

type updateProfileRequest struct {
	Username  *string `json:"username"`
	AvatarURL *string `json:"avatarUrl"`
}

func (d *AuthDelivery) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	log := logger.FromContext(r.Context())
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		utilities.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req updateProfileRequest
	if err := utilities.DecodeJSON(r, &req); err != nil {
		utilities.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if req.Username == nil && req.AvatarURL == nil {
		utilities.WriteError(w, http.StatusBadRequest, "nothing to update")
		return
	}

	user, err := d.Usecase.UpdateProfile(r.Context(), userID, req.Username, req.AvatarURL)
	if err != nil {
		log.Error().Err(err).Uint64("user_id", userID).Msg("failed to update profile")
		utilities.WriteError(w, utilities.StatusFromErr(err), err.Error())
		return
	}

	utilities.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}
