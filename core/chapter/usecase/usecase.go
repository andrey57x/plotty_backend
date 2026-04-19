package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	chapterrepo "github.com/fivecode/plotty/core/chapter/repository"
	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
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
	log := logger.FromContext(ctx)

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
		return nil, fmt.Errorf("chapter_uc.Create get story: %w", err)
	}
	ch, err := u.chapters.Create(ctx, storyID, title, content)
	if err != nil {
		log.Error().Err(err).Stringer("story_id", storyID).Msg("chapter_uc: create failed")
		return nil, fmt.Errorf("chapter_uc.Create: %w", err)
	}

	log.Info().Stringer("chapter_id", ch.ID).Stringer("story_id", storyID).Msg("chapter_uc: created")
	return ch, nil
}

func (u *Usecase) Update(ctx context.Context, id uuid.UUID, title *string, content *string) (*models.Chapter, error) {
	log := logger.FromContext(ctx)

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
	ch, err := u.chapters.Update(ctx, id, title, content)
	if err != nil {
		log.Error().Err(err).Stringer("chapter_id", id).Msg("chapter_uc: update failed")
		return nil, fmt.Errorf("chapter_uc.Update: %w", err)
	}

	if ch.Status == "published" && content != nil {
		briefs, errBriefs := u.chapters.ListBriefByStory(ctx, ch.StoryID)
		var prevChapterID string
		if errBriefs == nil {
			for _, b := range briefs {
				if b.ID == ch.ID {
					break
				}
				if b.Status == "published" {
					prevChapterID = b.ID.String()
				}
			}
		}

		loreTask := rabbitmq.MLTaskMessage{
			TaskID:  uuid.NewString(),
			TraceID: uuid.NewString(),
			Type:    "extract_lore",
			Payload: ch.Content,
			Metadata: map[string]string{
				"story_id":        ch.StoryID.String(),
				"chapter_id":      ch.ID.String(),
				"prev_chapter_id": prevChapterID,
			},
		}
		u.publishToRabbitMQ(ctx, loreTask)
	}

	log.Info().Stringer("chapter_id", id).Msg("chapter_uc: updated")
	return ch, nil
}

type ChapterWithImage struct {
	models.Chapter
	ImageURL *string
}

func (u *Usecase) Get(ctx context.Context, id uuid.UUID) (*ChapterWithImage, error) {
	ch, err := u.chapters.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("chapter_uc.Get: %w", err)
	}
	if ch.Status != "published" && u.authChecker != nil {
		if err := u.authChecker.CheckAuthorByChapter(ctx, id); err != nil {
			return nil, err
		}
	}
	imgURL, err := u.chapters.GetLatestImageURL(ctx, id)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Stringer("chapter_id", id).Msg("chapter_uc: get image url failed")
		return nil, fmt.Errorf("chapter_uc.Get image: %w", err)
	}
	return &ChapterWithImage{Chapter: *ch, ImageURL: imgURL}, nil
}

func (u *Usecase) Delete(ctx context.Context, id uuid.UUID) error {
	if u.authChecker != nil {
		if err := u.authChecker.CheckAuthorByChapter(ctx, id); err != nil {
			return err
		}
	}
	if err := u.chapters.Delete(ctx, id); err != nil {
		return fmt.Errorf("chapter_uc.Delete: %w", err)
	}
	logger.Ctx(ctx).Info().Stringer("chapter_id", id).Msg("chapter_uc: deleted")
	return nil
}

func (u *Usecase) Publish(ctx context.Context, chapterID uuid.UUID) error {
	log := logger.FromContext(ctx)

	if u.authChecker != nil {
		if err := u.authChecker.CheckAuthorByChapter(ctx, chapterID); err != nil {
			return err
		}
	}
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return fmt.Errorf("chapter_uc.Publish get chapter: %w", err)
	}

	if err := u.chapters.Publish(ctx, chapterID); err != nil {
		return fmt.Errorf("chapter_uc.Publish chapter: %w", err)
	}

	if err := u.stories.Publish(ctx, ch.StoryID); err != nil {
		log.Warn().Err(err).Stringer("story_id", ch.StoryID).Msg("chapter_uc: publish story failed (non-fatal)")
	}

	briefs, err := u.chapters.ListBriefByStory(ctx, ch.StoryID)
	if err != nil {
		log.Warn().Err(err).Stringer("story_id", ch.StoryID).Msg("chapter_uc: list briefs failed (non-fatal)")
	}

	var prevChapterID string
	publishedCount := 0
	for _, b := range briefs {
		if b.ID == chapterID {
			break
		}
		if b.Status == "published" {
			prevChapterID = b.ID.String()
			publishedCount++
		}
	}
	publishedCount++

	loreTask := rabbitmq.MLTaskMessage{
		TaskID:  uuid.NewString(),
		TraceID: uuid.NewString(),
		Type:    "extract_lore",
		Payload: ch.Content,
		Metadata: map[string]string{
			"story_id":        ch.StoryID.String(),
			"chapter_id":      chapterID.String(),
			"prev_chapter_id": prevChapterID,
		},
	}
	u.publishToRabbitMQ(ctx, loreTask)

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

	log.Info().Stringer("chapter_id", chapterID).Stringer("story_id", ch.StoryID).Msg("chapter_uc: published")
	return nil
}

func (u *Usecase) publishToRabbitMQ(ctx context.Context, task rabbitmq.MLTaskMessage) {
	log := logger.FromContext(ctx)
	body, err := json.Marshal(task)
	if err != nil {
		log.Error().Err(err).Str("task_type", task.Type).Msg("chapter_uc: failed to marshal ML task")
		return
	}
	if err := u.rmqChan.PublishWithContext(ctx, "", "ml_tasks_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	}); err != nil {
		log.Error().Err(err).Str("task_type", task.Type).Msg("chapter_uc: failed to publish ML task")
	} else {
		log.Info().Str("task_type", task.Type).Str("task_id", task.TaskID).Msg("chapter_uc: ML task published")
	}
}
