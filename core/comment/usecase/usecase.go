package usecase

import (
	"context"
	"strings"

	commentrepo "github.com/fivecode/plotty/core/comment/repository"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/google/uuid"
)

type Usecase struct {
	comments *commentrepo.Repository
}

func New(comments *commentrepo.Repository) *Usecase {
	return &Usecase{comments: comments}
}

func (u *Usecase) Create(ctx context.Context, chapterID uuid.UUID, content string) (*models.Comment, error) {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return nil, named_errors.ErrNoAccess
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, named_errors.ErrInvalidInput
	}
	if len(content) > 5000 {
		return nil, named_errors.ErrInvalidInput
	}
	return u.comments.Create(ctx, chapterID, userID, content)
}

func (u *Usecase) List(ctx context.Context, chapterID uuid.UUID, page, pageSize int) ([]models.Comment, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	return u.comments.ListByChapter(ctx, chapterID, pageSize, offset)
}

func (u *Usecase) Delete(ctx context.Context, commentID uuid.UUID) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	ownerID, err := u.comments.GetOwnerID(ctx, commentID)
	if err != nil {
		return named_errors.ErrNotFound
	}
	if ownerID != userID {
		return named_errors.ErrNoAccess
	}
	return u.comments.Delete(ctx, commentID)
}
