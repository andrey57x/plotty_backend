package usecase

import (
	"context"
	"fmt"

	"github.com/fivecode/plotty/core/logger"
	likerepo "github.com/fivecode/plotty/core/like/repository"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/google/uuid"
)

type Usecase struct {
	likes *likerepo.Repository
}

func New(likes *likerepo.Repository) *Usecase {
	return &Usecase{likes: likes}
}

func (u *Usecase) Like(ctx context.Context, storyID uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	if err := u.likes.Like(ctx, userID, storyID); err != nil {
		return fmt.Errorf("like_uc.Like: %w", err)
	}
	logger.Ctx(ctx).Info().Uint64("user_id", userID).Stringer("story_id", storyID).Msg("like_uc: liked")
	return nil
}

func (u *Usecase) Unlike(ctx context.Context, storyID uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	if err := u.likes.Unlike(ctx, userID, storyID); err != nil {
		return fmt.Errorf("like_uc.Unlike: %w", err)
	}
	logger.Ctx(ctx).Info().Uint64("user_id", userID).Stringer("story_id", storyID).Msg("like_uc: unliked")
	return nil
}

func (u *Usecase) Status(ctx context.Context, storyID uuid.UUID) (int, bool, error) {
	count, err := u.likes.Count(ctx, storyID)
	if err != nil {
		return 0, false, fmt.Errorf("like_uc.Status count: %w", err)
	}
	var liked bool
	if userID, ok := middleware.GetUserID(ctx); ok {
		liked, _ = u.likes.IsLiked(ctx, userID, storyID)
	}
	return count, liked, nil
}
