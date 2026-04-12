package usecase

import (
	"context"

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
	return u.likes.Like(ctx, userID, storyID)
}

func (u *Usecase) Unlike(ctx context.Context, storyID uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	return u.likes.Unlike(ctx, userID, storyID)
}

func (u *Usecase) Status(ctx context.Context, storyID uuid.UUID) (int, bool, error) {
	count, err := u.likes.Count(ctx, storyID)
	if err != nil {
		return 0, false, err
	}
	var liked bool
	if userID, ok := middleware.GetUserID(ctx); ok {
		liked, _ = u.likes.IsLiked(ctx, userID, storyID)
	}
	return count, liked, nil
}
