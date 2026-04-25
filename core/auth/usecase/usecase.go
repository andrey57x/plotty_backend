package usecase

import (
	"context"
	"fmt"

	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/models"
	namederrors "github.com/fivecode/plotty/core/named_errors"
	"golang.org/x/crypto/bcrypt"
)

type AuthUsecase struct {
	Repository AuthRepository
}

type AuthRepository interface {
	CreateSession(ctx context.Context, userID uint64) (string, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	DeleteSession(ctx context.Context, sessionID string) error
	GetUserIDBySession(ctx context.Context, sessionID string) (uint64, error)
	CreateUser(ctx context.Context, email, passwordHash string) (*models.User, error)
	GetUserByID(ctx context.Context, userID uint64) (*models.User, error)
	UpdateUser(ctx context.Context, userID uint64, username *string, avatarURL *string, bio *string) (*models.User, error)
	GetPublicProfileByUsername(ctx context.Context, username string) (*models.PublicUserProfile, error)
}

func New(repo AuthRepository) *AuthUsecase {
	return &AuthUsecase{
		Repository: repo,
	}
}

func (u *AuthUsecase) Login(ctx context.Context, email, password string) (*models.User, string, error) {
	log := logger.FromContext(ctx)
	log.Info().Str("email", email).Msg("user login attempt")

	user, err := u.Repository.GetUserByEmail(ctx, email)
	if err != nil {
		log.Warn().Str("email", email).Msg("user not found")
		return nil, "", namederrors.ErrInvalidEmailOrPassword
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		log.Warn().Str("email", email).Msg("invalid password")
		return nil, "", namederrors.ErrInvalidEmailOrPassword
	}

	sessionID, err := u.Repository.CreateSession(ctx, user.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to create session")
		return nil, "", fmt.Errorf("failed to create session: %w", err)
	}

	log.Info().Uint64("user_id", user.ID).Msg("user logged in successfully")
	return user, sessionID, nil
}

func (u *AuthUsecase) Register(ctx context.Context, email, password string) (*models.User, string, error) {
	log := logger.FromContext(ctx)
	log.Info().Str("email", email).Msg("user registration attempt")

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("failed to hash password")
		return nil, "", fmt.Errorf("failed to hash password: %w", err)
	}

	user, err := u.Repository.CreateUser(ctx, email, string(hashedPassword))
	if err != nil {
		log.Error().Err(err).Str("email", email).Msg("failed to create user")
		return nil, "", err
	}

	sessionID, err := u.Repository.CreateSession(ctx, user.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to create session")
		return nil, "", fmt.Errorf("failed to create session: %w", err)
	}

	log.Info().Uint64("user_id", user.ID).Msg("user registered successfully")
	return user, sessionID, nil
}

func (u *AuthUsecase) Logout(ctx context.Context, sessionID string) error {
	log := logger.FromContext(ctx)
	log.Info().Str("session_id", sessionID).Msg("user logout")

	if err := u.Repository.DeleteSession(ctx, sessionID); err != nil {
		log.Error().Err(err).Msg("failed to delete session")
		return fmt.Errorf("failed to logout: %w", err)
	}

	return nil
}

func (u *AuthUsecase) UpdateProfile(ctx context.Context, userID uint64, username *string, avatarURL *string, bio *string) (*models.User, error) {
	log := logger.FromContext(ctx)

	user, err := u.Repository.UpdateUser(ctx, userID, username, avatarURL, bio)
	if err != nil {
		log.Error().Err(err).Uint64("user_id", userID).Msg("auth_uc: update profile failed")
		return nil, fmt.Errorf("auth_uc.UpdateProfile: %w", err)
	}

	log.Info().Uint64("user_id", userID).Msg("auth_uc: profile updated")
	return user, nil
}

func (u *AuthUsecase) GetPublicProfileByUsername(ctx context.Context, username string) (*models.PublicUserProfile, error) {
	return u.Repository.GetPublicProfileByUsername(ctx, username)
}

func (u *AuthUsecase) GetUserBySession(ctx context.Context, sessionID string) (*models.User, error) {
	log := logger.FromContext(ctx)

	userID, err := u.Repository.GetUserIDBySession(ctx, sessionID)
	if err != nil {
		log.Warn().Err(err).Msg("auth_uc: get user id by session failed")
		return nil, fmt.Errorf("auth_uc.GetUserBySession session: %w", err)
	}

	user, err := u.Repository.GetUserByID(ctx, userID)
	if err != nil {
		log.Error().Err(err).Uint64("user_id", userID).Msg("failed to get user by id")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}
