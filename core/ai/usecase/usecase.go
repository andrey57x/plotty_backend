package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/fivecode/plotty/core/ai/repository"
	chapterrepo "github.com/fivecode/plotty/core/chapter/repository"
	"github.com/fivecode/plotty/core/constants"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	storyrepo "github.com/fivecode/plotty/core/story/repository"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Usecase struct {
	jobs     *repository.Repository
	chapters *chapterrepo.Repository
	stories  *storyrepo.Repository
	rmqChan  *amqp.Channel
}

func New(jobs *repository.Repository, chapters *chapterrepo.Repository, stories *storyrepo.Repository, rmqChan *amqp.Channel) *Usecase {
	return &Usecase{jobs: jobs, chapters: chapters, stories: stories, rmqChan: rmqChan}
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

func (u *Usecase) StartSpellcheck(ctx context.Context, chapterID uuid.UUID, content string) (uuid.UUID, error) {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return uuid.Nil, err
	}
	text := strings.TrimSpace(content)
	if text == "" {
		return uuid.Nil, named_errors.ErrInvalidInput
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
	}
	body, _ := json.Marshal(task)
	_ = u.rmqChan.PublishWithContext(ctx, "", "spellcheck_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})

	return jobID, nil
}

func (u *Usecase) StartImageGeneration(ctx context.Context, chapterID uuid.UUID, content, prompt string) (uuid.UUID, error) {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return uuid.Nil, err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return uuid.Nil, named_errors.ErrInvalidInput
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
	_ = u.rmqChan.PublishWithContext(ctx, "", "ml_tasks_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})

	return jobID, nil
}

func (u *Usecase) ProcessMLResult(ctx context.Context, res rabbitmq.MLResultMessage) error {
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

func (u *Usecase) StartLogicCheck(ctx context.Context, chapterID uuid.UUID, content string) (uuid.UUID, error) {
	ch, err := u.chapters.GetByID(ctx, chapterID)
	if err != nil {
		return uuid.Nil, err
	}
	text := strings.TrimSpace(content)
	if text == "" {
		return uuid.Nil, named_errors.ErrInvalidInput
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
			"story_id": ch.StoryID.String(),
		},
	}
	body, _ := json.Marshal(task)
	_ = u.rmqChan.PublishWithContext(ctx, "", "ml_tasks_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})

	return jobID, nil
}
