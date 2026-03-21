package usecase

import (
	"context"

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
	return u.repo.List(ctx, category)
}
