package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fivecode/plotty/core/ai/repository"
	chapterrepo "github.com/fivecode/plotty/core/chapter/repository"
	"github.com/fivecode/plotty/core/constants"
	"github.com/fivecode/plotty/core/logger"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type CreditsHandler interface {
	DeductCredits(ctx context.Context, userID uint64, amount int, jobType string) error
	RefundCredits(ctx context.Context, userID uint64, amount int, jobType string) error
}

type Usecase struct {
	jobs     *repository.Repository
	chapters *chapterrepo.Repository
	stories  *storyrepo.Repository
	rmqChan  *amqp.Channel
	credits  CreditsHandler
}

func New(jobs *repository.Repository, chapters *chapterrepo.Repository, stories *storyrepo.Repository, rmqChan *amqp.Channel, credits CreditsHandler) *Usecase {
	return &Usecase{jobs: jobs, chapters: chapters, stories: stories, rmqChan: rmqChan, credits: credits}
}

func sha256Hex(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

type spellcheckInput struct {
	ChapterID string `json:"chapterId"`
	Content   string `json:"content"`
}

type imageGenInput struct {
	ChapterID string `json:"chapterId"`
	Content   string `json:"content"`
	Prompt    string `json:"prompt"`
}

func (u *Usecase) StartSpellcheck(ctx context.Context, userID uint64, chapterID uuid.UUID, content string) (uuid.UUID, error) {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return uuid.Nil, err
	}
	text := strings.TrimSpace(content)
	if text == "" {
		return uuid.Nil, named_errors.ErrInvalidInput
	}

	contentHash := sha256Hex(text)

	if cachedJob, err := u.jobs.GetCompletedJobByHash(ctx, chapterID, constants.AIJobTypeSpellcheck, contentHash); err == nil {
		return cachedJob.ID, nil
	}

	tags, err := u.stories.TagsForStory(ctx, ch.StoryID)
	var fandomSlug string
	if err == nil {
		for _, t := range tags {
			if t.Category == "directionality" && t.Slug != "originals" {
				fandomSlug = t.Slug
			}
		}
	}

	payloadBytes, _ := json.Marshal(spellcheckInput{ChapterID: chapterID.String(), Content: text})
	jobID := uuid.New()
	now := time.Now().UTC()

	job := models.AIJob{
		ID:           jobID,
		Type:         constants.AIJobTypeSpellcheck,
		Status:       constants.AIJobStatusProcessing,
		ChapterID:    &ch.ID,
		StoryID:      &ch.StoryID,
		InputPayload: payloadBytes,
		ContentHash:  &contentHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := u.jobs.CreateJob(ctx, job); err != nil {
		return uuid.Nil, err
	}

	task := rabbitmq.MLTaskMessage{
		TaskID:  jobID.String(),
		Type:    "spellcheck",
		Payload: string(payloadBytes),
		Metadata: map[string]string{
			"story_id":    ch.StoryID.String(),
			"fandom_slug": fandomSlug,
		},
	}
	body, _ := json.Marshal(task)
	_ = u.rmqChan.PublishWithContext(ctx, "", "spellcheck_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})

	return jobID, nil
}

func (u *Usecase) StartImageGeneration(ctx context.Context, userID uint64, chapterID uuid.UUID, content, prompt string) (uuid.UUID, error) {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return uuid.Nil, err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return uuid.Nil, named_errors.ErrInvalidInput
	}

	contentHash := sha256Hex(content + "|" + prompt)

	if err := u.credits.DeductCredits(ctx, userID, constants.CreditCostImageGen, constants.AIJobTypeImageGeneration); err != nil {
		return uuid.Nil, err
	}

	payloadBytes, _ := json.Marshal(imageGenInput{
		ChapterID: chapterID.String(),
		Content:   content,
		Prompt:    prompt,
	})
	jobID := uuid.New()
	now := time.Now().UTC()

	job := models.AIJob{
		ID:           jobID,
		Type:         constants.AIJobTypeImageGeneration,
		Status:       constants.AIJobStatusProcessing,
		ChapterID:    &ch.ID,
		StoryID:      &ch.StoryID,
		InputPayload: payloadBytes,
		ContentHash:  &contentHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := u.jobs.CreateJob(ctx, job); err != nil {
		return uuid.Nil, err
	}

	task := rabbitmq.MLTaskMessage{
		TaskID:  jobID.String(),
		Type:    "image_gen",
		Payload: string(payloadBytes),
	}
	body, _ := json.Marshal(task)
	_ = u.rmqChan.PublishWithContext(ctx, "", "ml_image_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})

	return jobID, nil
}

func (u *Usecase) StartLogicCheck(ctx context.Context, userID uint64, chapterID uuid.UUID, content string) (uuid.UUID, error) {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return uuid.Nil, err
	}

	if err := u.credits.DeductCredits(ctx, userID, constants.CreditCostLogicCheck, constants.AIJobTypeLogicCheck); err != nil {
		return uuid.Nil, err
	}

	text := strings.TrimSpace(content)
	if text == "" {
		return uuid.Nil, named_errors.ErrInvalidInput
	}

	contentHash := sha256Hex(text)

	if cachedJob, err := u.jobs.GetCompletedJobByHash(ctx, chapterID, constants.AIJobTypeLogicCheck, contentHash); err == nil {
		return cachedJob.ID, nil
	}

	briefs, _ := u.chapters.ListBriefByStory(ctx, ch.StoryID)
	var prevIDs []string
	for _, b := range briefs {
		if b.ID == chapterID {
			break
		}
		if b.Status == "published" {
			prevIDs = append(prevIDs, b.ID.String())
		}
	}

	jobID := uuid.New()
	now := time.Now().UTC()

	job := models.AIJob{
		ID:           jobID,
		Type:         constants.AIJobTypeLogicCheck,
		Status:       constants.AIJobStatusProcessing,
		ChapterID:    &ch.ID,
		StoryID:      &ch.StoryID,
		InputPayload: []byte(`{"chapterId":"` + chapterID.String() + `"}`),
		ContentHash:  &contentHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := u.jobs.CreateJob(ctx, job); err != nil {
		return uuid.Nil, err
	}

	task := rabbitmq.MLTaskMessage{
		TaskID:  jobID.String(),
		TraceID: uuid.NewString(),
		Type:    "logic_check",
		Payload: text,
		Metadata: map[string]string{
			"story_id":         ch.StoryID.String(),
			"prev_chapter_ids": strings.Join(prevIDs, ","),
		},
	}
	body, _ := json.Marshal(task)
	_ = u.rmqChan.PublishWithContext(ctx, "", "ml_tasks_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})

	return jobID, nil
}

func (u *Usecase) ProcessMLResult(ctx context.Context, res rabbitmq.MLResultMessage) error {
	log := logger.FromContext(ctx)

	if res.Type == "generate_summary" && res.Status == "completed" {
		storyIDStr, ok := res.Metadata["story_id"]
		if !ok {
			return errors.New("story_id missing in summary metadata")
		}
		storyID, err := uuid.Parse(storyIDStr)
		if err != nil {
			return err
		}

		var summaryData map[string]string
		if err := json.Unmarshal(res.Result, &summaryData); err == nil {
			if summaryText, ok := summaryData["summary"]; ok {
				return u.stories.UpdateAISummary(ctx, storyID, summaryText)
			}
		}
		return nil
	}

	if res.Type == "extract_lore" {
		return nil
	}

	taskID, err := uuid.Parse(res.TaskID)
	if err != nil {
		return err
	}

	job, err := u.jobs.GetJob(ctx, taskID)
	if err != nil {
		return err
	}

	var errMsg *string
	if res.Error != nil {
		msg := res.Error.Message
		errMsg = &msg
	}

	if err := u.jobs.UpdateJob(ctx, taskID, res.Status, res.Result, errMsg); err != nil {
		return err
	}

	// Сохранение и логирование потраченных токенов GigaChat
	if res.PromptTokens > 0 || res.CompletionTokens > 0 {
		log.Info().
			Str("job_id", res.TaskID).
			Str("job_type", job.Type).
			Int("prompt_tokens", res.PromptTokens).
			Int("completion_tokens", res.CompletionTokens).
			Int("total_tokens", res.TotalTokens).
			Msg("Логирование токенов в бэкенде")
		_ = u.jobs.UpdateJobTokens(ctx, taskID, res.PromptTokens, res.CompletionTokens, res.TotalTokens)
	}

	// Обработка возврата кредитов в случае падения платной задачи
	if res.Status == constants.AIJobStatusFailed {
		var refundAmount int
		switch job.Type {
		case constants.AIJobTypeImageGeneration:
			refundAmount = constants.CreditCostImageGen
		case constants.AIJobTypeLogicCheck:
			refundAmount = constants.CreditCostLogicCheck
		case "canon_check":
			refundAmount = constants.CreditCostCanonCheck
		}

		if refundAmount > 0 && job.StoryID != nil {
			if story, err := u.stories.GetByID(ctx, *job.StoryID); err == nil && story != nil && story.AuthorID != nil {
				_ = u.credits.RefundCredits(ctx, *story.AuthorID, refundAmount, job.Type)
			}
		}
	}

	if res.Status == constants.AIJobStatusCompleted && job.Type == constants.AIJobTypeImageGeneration {
		var imgRes struct {
			URL    string `json:"url"`
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(res.Result, &imgRes); err == nil {
			gimg := models.GeneratedImage{
				ID:        uuid.New(),
				JobID:     taskID,
				ChapterID: job.ChapterID,
				Prompt:    imgRes.Prompt,
				ImageURL:  imgRes.URL,
				CreatedAt: time.Now().UTC(),
			}
			_ = u.jobs.InsertGeneratedImage(ctx, gimg)
		}
	}

	return nil
}

func (u *Usecase) GetJobView(ctx context.Context, jobID uuid.UUID) (map[string]any, error) {
	j, err := u.jobs.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"jobId":  j.ID.String(),
		"type":   j.Type,
		"status": j.Status,
	}
	if j.ErrorMessage != nil {
		out["error"] = *j.ErrorMessage
	}
	if j.Status == constants.AIJobStatusCompleted && len(j.ResultPayload) > 0 {
		if j.Type == constants.AIJobTypeImageGeneration {
			imgs, err := u.jobs.ListImagesByJob(ctx, j.ID)
			if err != nil {
				return nil, err
			}
			merged, err := repository.MergeResultPayloadImages(j.ResultPayload, imgs)
			if err != nil {
				return nil, err
			}
			var res any
			if err := json.Unmarshal(merged, &res); err != nil {
				return nil, err
			}
			out["result"] = res
		} else {
			var res any
			if err := json.Unmarshal(j.ResultPayload, &res); err != nil {
				return nil, err
			}
			out["result"] = res
		}
	}
	return out, nil
}

func (u *Usecase) StartCanonCheck(ctx context.Context, userID uint64, chapterID uuid.UUID) (uuid.UUID, error) {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return uuid.Nil, err
	}

	if err := u.credits.DeductCredits(ctx, userID, constants.CreditCostCanonCheck, "canon_check"); err != nil {
		return uuid.Nil, err
	}

	tags, err := u.stories.TagsForStory(ctx, ch.StoryID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to get tags: %w", err)
	}

	var fandomSlug string
	var warnings []string

	for _, t := range tags {
		if t.Category == "directionality" && t.Slug != "originals" {
			fandomSlug = t.Slug
		}
		if t.Category == "warning" {
			warnings = append(warnings, t.Slug)
		}
	}

	if fandomSlug == "" {
		return uuid.Nil, errors.New("story is original or has no fandom tag, canon check is not applicable")
	}

	jobID := uuid.New()
	now := time.Now().UTC()
	contentHash := sha256Hex(ch.DraftContent)

	job := models.AIJob{
		ID:           jobID,
		Type:         "canon_check",
		Status:       constants.AIJobStatusProcessing,
		ChapterID:    &ch.ID,
		StoryID:      &ch.StoryID,
		InputPayload: []byte(`{"chapterId":"` + chapterID.String() + `"}`),
		ContentHash:  &contentHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := u.jobs.CreateJob(ctx, job); err != nil {
		return uuid.Nil, err
	}

	task := rabbitmq.MLTaskMessage{
		TaskID:  jobID.String(),
		TraceID: uuid.NewString(),
		Type:    "canon_check",
		Payload: ch.DraftContent,
		Metadata: map[string]string{
			"story_id":    ch.StoryID.String(),
			"fandom_slug": fandomSlug,
			"warnings":    strings.Join(warnings, ","),
		},
	}

	body, _ := json.Marshal(task)
	_ = u.rmqChan.PublishWithContext(ctx, "", "ml_tasks_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})

	return jobID, nil
}
