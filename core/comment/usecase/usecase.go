package usecase

import (
	"context"
	"fmt"
	"strings"

	commentrepo "github.com/fivecode/plotty/core/comment/repository"
	"github.com/fivecode/plotty/core/logger"
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
	log := logger.FromContext(ctx)

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		log.Warn().Stringer("chapter_id", chapterID).Msg("comment_uc: create without auth")
		return nil, named_errors.ErrNoAccess
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, named_errors.ErrInvalidInput
	}
	if len(content) > 5000 {
		return nil, named_errors.ErrInvalidInput
	}

	c, err := u.comments.Create(ctx, chapterID, userID, content)
	if err != nil {
		log.Error().Err(err).Stringer("chapter_id", chapterID).Uint64("user_id", userID).Msg("comment_uc: create failed")
		return nil, fmt.Errorf("comment_uc.Create: %w", err)
	}
	log.Info().Stringer("comment_id", c.ID).Stringer("chapter_id", chapterID).Msg("comment_uc: created")
	return c, nil
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

	comments, total, err := u.comments.ListByChapter(ctx, chapterID, pageSize, offset)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("chapter_id", chapterID).Msg("comment_uc: list failed")
		return nil, 0, fmt.Errorf("comment_uc.List: %w", err)
	}
	return comments, total, nil
}

func (u *Usecase) Delete(ctx context.Context, commentID uuid.UUID) error {
	log := logger.FromContext(ctx)

	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	ownerID, err := u.comments.GetOwnerID(ctx, commentID)
	if err != nil {
		log.Warn().Err(err).Stringer("comment_id", commentID).Msg("comment_uc: get owner failed")
		return named_errors.ErrNotFound
	}
	if ownerID != userID {
		log.Warn().Uint64("user_id", userID).Uint64("owner_id", ownerID).Stringer("comment_id", commentID).Msg("comment_uc: delete denied, not owner")
		return named_errors.ErrNoAccess
	}
	if err := u.comments.Delete(ctx, commentID); err != nil {
		return fmt.Errorf("comment_uc.Delete: %w", err)
	}
	log.Info().Stringer("comment_id", commentID).Uint64("user_id", userID).Msg("comment_uc: deleted")
	return nil
}
