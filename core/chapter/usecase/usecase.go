package usecase

import (
	"context"
	"strings"

	chapterrepo "github.com/fivecode/plotty/core/chapter/repository"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	"github.com/google/uuid"
)

type Usecase struct {
	chapters *chapterrepo.Repository
	stories  *storyrepo.Repository
}

func New(chapters *chapterrepo.Repository, stories *storyrepo.Repository) *Usecase {
	return &Usecase{chapters: chapters, stories: stories}
}

func (u *Usecase) Create(ctx context.Context, storyID uuid.UUID, title, content string) (*models.Chapter, error) {
	title = strings.TrimSpace(title)
	if title == "" || strings.TrimSpace(content) == "" {
		return nil, named_errors.ErrInvalidInput
	}
	if _, err := u.stories.GetByID(ctx, storyID); err != nil {
		return nil, err
	}
	return u.chapters.Create(ctx, storyID, title, content)
}

func (u *Usecase) Update(ctx context.Context, id uuid.UUID, title *string, content *string) (*models.Chapter, error) {
	if content != nil {
		c := strings.TrimSpace(*content)
		if c == "" {
			return nil, named_errors.ErrInvalidInput
		}
		content = &c
	}
	if title != nil {
		t := strings.TrimSpace(*title)
		if t == "" {
			return nil, named_errors.ErrInvalidInput
		}
		title = &t
	}
	return u.chapters.Update(ctx, id, title, content)
}

func (u *Usecase) Get(ctx context.Context, id uuid.UUID) (*models.Chapter, error) {
	return u.chapters.GetByID(ctx, id)
}

func (u *Usecase) Delete(ctx context.Context, id uuid.UUID) error {
	return u.chapters.Delete(ctx, id)
}
