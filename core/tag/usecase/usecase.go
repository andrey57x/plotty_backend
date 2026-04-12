package usecase

import (
	"context"
	"fmt"

	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/models"
	tagrepo "github.com/fivecode/plotty/core/tag/repository"
)

type Usecase struct {
	repo *tagrepo.Repository
}

func New(repo *tagrepo.Repository) *Usecase {
	return &Usecase{repo: repo}
}

func (u *Usecase) List(ctx context.Context, category string) ([]models.Tag, error) {
	tags, err := u.repo.List(ctx, category)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("category", category).Msg("tag_uc: list failed")
		return nil, fmt.Errorf("tag_uc.List: %w", err)
	}
	return tags, nil
}
