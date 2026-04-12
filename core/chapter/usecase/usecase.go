package usecase

import (
	"context"
	"encoding/json"
	"strings"

	chapterrepo "github.com/fivecode/plotty/core/chapter/repository"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

type StoryAuthorChecker interface {
	CheckAuthorByStory(ctx context.Context, storyID uuid.UUID) error
	CheckAuthorByChapter(ctx context.Context, chapterID uuid.UUID) error
}

type Usecase struct {
	chapters    *chapterrepo.Repository
	stories     *storyrepo.Repository
	rmqChan     *amqp.Channel
	authChecker StoryAuthorChecker
}

func New(chapters *chapterrepo.Repository, stories *storyrepo.Repository, rmqChan *amqp.Channel) *Usecase {
	return &Usecase{chapters: chapters, stories: stories, rmqChan: rmqChan}
}

func (u *Usecase) SetAuthorChecker(checker StoryAuthorChecker) {
	u.authChecker = checker
}

func (u *Usecase) Create(ctx context.Context, storyID uuid.UUID, title, content string) (*models.Chapter, error) {
	if u.authChecker != nil {
		if err := u.authChecker.CheckAuthorByStory(ctx, storyID); err != nil {
			return nil, err
		}
	}
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
	if u.authChecker != nil {
		if err := u.authChecker.CheckAuthorByChapter(ctx, id); err != nil {
			return nil, err
		}
	}
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

type ChapterWithImage struct {
	models.Chapter
	ImageURL *string
}

func (u *Usecase) Get(ctx context.Context, id uuid.UUID) (*ChapterWithImage, error) {
	ch, err := u.chapters.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	imgURL, err := u.chapters.GetLatestImageURL(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ChapterWithImage{Chapter: *ch, ImageURL: imgURL}, nil
}

func (u *Usecase) Delete(ctx context.Context, id uuid.UUID) error {
	if u.authChecker != nil {
		if err := u.authChecker.CheckAuthorByChapter(ctx, id); err != nil {
			return err
		}
	}
	return u.chapters.Delete(ctx, id)
}

func (u *Usecase) Publish(ctx context.Context, chapterID uuid.UUID) error {
	if u.authChecker != nil {
		if err := u.authChecker.CheckAuthorByChapter(ctx, chapterID); err != nil {
			return err
		}
	}
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return err
	}

	if ch.Status == "published" {
		return nil
	}

	// 1. Публикуем главу
	if err := u.chapters.Publish(ctx, chapterID); err != nil {
		return err
	}

	// 2. Публикуем историю (состояние обновится, если она была в draft)
	_ = u.stories.Publish(ctx, ch.StoryID)

	// 3. Отправляем задачу в ML: "Извлечь лор (Story State)"
	loreTask := rabbitmq.MLTaskMessage{
		TaskID:  uuid.NewString(),
		TraceID: uuid.NewString(), // В будущем можно брать из логгера
		Type:    "extract_lore",
		Payload: ch.Content,
		Metadata: map[string]string{
			"story_id":   ch.StoryID.String(),
			"chapter_id": chapterID.String(),
		},
	}
	u.publishToRabbitMQ(ctx, loreTask)

	// 4. Логика генерации Summary:
	briefs, _ := u.chapters.ListBriefByStory(ctx, ch.StoryID)
	publishedCount := 0
	for _, b := range briefs {
		if b.Status == "published" {
			publishedCount++
		}
	}

	if publishedCount == 1 {
		summaryTask := rabbitmq.MLTaskMessage{
			TaskID:  uuid.NewString(),
			TraceID: uuid.NewString(),
			Type:    "generate_summary",
			Payload: ch.Content,
			Metadata: map[string]string{
				"story_id": ch.StoryID.String(),
			},
		}
		u.publishToRabbitMQ(ctx, summaryTask)
	}

	return nil
}

func (u *Usecase) publishToRabbitMQ(ctx context.Context, task rabbitmq.MLTaskMessage) {
	body, _ := json.Marshal(task)
	err := u.rmqChan.PublishWithContext(ctx, "", "ml_tasks_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
	if err != nil {
		log.Error().Err(err).Str("task_type", task.Type).Msg("failed to publish ML task")
	}
}
