package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fivecode/plotty/core/constants"
	fandomrepo "github.com/fivecode/plotty/core/fandom/repository"
	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/middleware"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/fivecode/plotty/core/slug"
	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type TagManager interface {
	ExistsByName(ctx context.Context, name string) (bool, error)
	Create(ctx context.Context, t models.Tag) error
}

type UserChecker interface {
	GetUserByID(ctx context.Context, userID uint64) (*models.User, error)
}

type Usecase struct {
	repo    *fandomrepo.Repository
	tags    TagManager
	users   UserChecker
	rmqChan *amqp.Channel
}

func New(repo *fandomrepo.Repository, tags TagManager, users UserChecker, rmqChan *amqp.Channel) *Usecase {
	return &Usecase{repo: repo, tags: tags, users: users, rmqChan: rmqChan}
}

func (u *Usecase) checkAdmin(ctx context.Context) error {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return named_errors.ErrNoAccess
	}
	user, err := u.users.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !user.IsAdmin {
		logger.Ctx(ctx).Warn().Uint64("user_id", userID).Msg("fandom_uc: admin access denied")
		return named_errors.ErrNoAccess
	}
	return nil
}

func (u *Usecase) Suggest(ctx context.Context, name, description string) (*models.SuggestedFandom, error) {
	userID, ok := middleware.GetUserID(ctx)
	if !ok {
		return nil, named_errors.ErrNoAccess
	}

	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)

	if name == "" || description == "" {
		return nil, named_errors.ErrInvalidInput
	}
	if len(description) > 3000 {
		return nil, fmt.Errorf("%w: description too long (max 3000)", named_errors.ErrInvalidInput)
	}

	exists, err := u.tags.ExistsByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("%w: fandom already exists", named_errors.ErrConflict)
	}

	now := time.Now().UTC()
	f := models.SuggestedFandom{
		ID:          uuid.New(),
		UserID:      userID,
		Name:        name,
		Description: description,
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := u.repo.Create(ctx, f); err != nil {
		return nil, err
	}

	logger.Ctx(ctx).Info().Stringer("id", f.ID).Str("name", name).Msg("fandom_uc: suggestion created")
	return &f, nil
}

func (u *Usecase) ListPending(ctx context.Context, page, pageSize int) ([]models.SuggestedFandom, error) {
	if err := u.checkAdmin(ctx); err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	return u.repo.ListPending(ctx, pageSize, offset)
}

func (u *Usecase) Approve(ctx context.Context, id uuid.UUID) error {
	if err := u.checkAdmin(ctx); err != nil {
		return err
	}

	fandom, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if fandom.Status != "pending" {
		return named_errors.ErrInvalidInput
	}

	// 1. Создаем тег
	fandomSlug := slug.FromTitle(fandom.Name)
	newTag := models.Tag{
		ID:       uuid.New(),
		Category: constants.TagCategoryDirectionality,
		Slug:     fandomSlug,
		Name:     fandom.Name,
	}

	if err := u.tags.Create(ctx, newTag); err != nil {
		return fmt.Errorf("fandom_uc.Approve create tag: %w", err)
	}

	// 2. Меняем статус
	if err := u.repo.UpdateStatus(ctx, id, "approved"); err != nil {
		return err
	}

	// 3. Отправляем в RabbitMQ задачу на генерацию лора
	task := sharedrmq.MLTaskMessage{
		TaskID:  uuid.NewString(),
		TraceID: uuid.NewString(),
		Type:    "generate_fandom_lore",
		Payload: fandom.Description,
		Metadata: map[string]string{
			"fandom_slug": fandomSlug,
		},
	}

	body, _ := json.Marshal(task)
	_ = u.rmqChan.PublishWithContext(ctx, "", "ml_tasks_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})

	logger.Ctx(ctx).Info().Stringer("id", id).Str("slug", fandomSlug).Msg("fandom_uc: approved & task sent to ML")
	return nil
}

func (u *Usecase) Reject(ctx context.Context, id uuid.UUID) error {
	if err := u.checkAdmin(ctx); err != nil {
		return err
	}
	return u.repo.UpdateStatus(ctx, id, "rejected")
}
