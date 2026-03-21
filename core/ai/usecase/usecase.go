package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fivecode/plotty/core/ai/repository"
	chapterrepo "github.com/fivecode/plotty/core/chapter/repository"
	"github.com/fivecode/plotty/core/constants"
	"github.com/fivecode/plotty/core/ml"
	"github.com/fivecode/plotty/core/models"
	"github.com/fivecode/plotty/core/named_errors"
	"github.com/google/uuid"
)

type Usecase struct {
	jobs     *repository.Repository
	chapters *chapterrepo.Repository
	ml       *ml.Client
}

func New(jobs *repository.Repository, chapters *chapterrepo.Repository, mlc *ml.Client) *Usecase {
	return &Usecase{jobs: jobs, chapters: chapters, ml: mlc}
}

func imageURLForDB(binaryOrURL string) (string, error) {
	s := strings.TrimSpace(binaryOrURL)
	if s == "" {
		return "", fmt.Errorf("empty image payload")
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s, nil
	}
	if strings.HasPrefix(s, "data:") {
		return s, nil
	}
	if _, err := base64.StdEncoding.DecodeString(s); err != nil {
		return "", fmt.Errorf("image must be URL, data URL, or raw base64: %w", err)
	}
	return "data:image/png;base64," + s, nil
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
	payload, _ := json.Marshal(spellcheckInput{ChapterID: chapterID.String(), Content: text})
	jobID := uuid.New()
	now := time.Now().UTC()
	job := models.AIJob{
		ID:           jobID,
		Type:         constants.AIJobTypeSpellcheck,
		Status:       constants.AIJobStatusQueued,
		ChapterID:    &ch.ID,
		StoryID:      &ch.StoryID,
		InputPayload: payload,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := u.jobs.CreateJob(ctx, job); err != nil {
		return uuid.Nil, err
	}
	go u.runSpellcheckJob(jobID, text)
	return jobID, nil
}

func (u *Usecase) runSpellcheckJob(jobID uuid.UUID, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusProcessing, nil, nil)

	res, err := u.ml.Spellcheck(ctx, text)
	if err != nil {
		msg := err.Error()
		_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusFailed, nil, &msg)
		return
	}
	raw, err := json.Marshal(res)
	if err != nil {
		msg := err.Error()
		_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusFailed, nil, &msg)
		return
	}
	_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusCompleted, raw, nil)
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
	payload, _ := json.Marshal(imageGenInput{
		ChapterID: chapterID.String(),
		Content:   content,
		Prompt:    prompt,
	})
	jobID := uuid.New()
	now := time.Now().UTC()
	job := models.AIJob{
		ID:           jobID,
		Type:         constants.AIJobTypeImageGeneration,
		Status:       constants.AIJobStatusQueued,
		ChapterID:    &ch.ID,
		StoryID:      &ch.StoryID,
		InputPayload: payload,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := u.jobs.CreateJob(ctx, job); err != nil {
		return uuid.Nil, err
	}
	go u.runImageJob(jobID, ch.ID, content, prompt)
	return jobID, nil
}

func (u *Usecase) runImageJob(jobID, chapterID uuid.UUID, content, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusProcessing, nil, nil)

	gen, err := u.ml.GenerateImage(ctx, content, prompt)
	if err != nil {
		msg := err.Error()
		_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusFailed, nil, &msg)
		return
	}

	type imgRow struct {
		ID       string `json:"id"`
		ImageURL string `json:"imageUrl"`
		Prompt   string `json:"prompt"`
	}
	var apiImages []imgRow
	for _, im := range gen.Images {
		imgID := uuid.New()
		url, serr := imageURLForDB(im.BinaryOrURL)
		if serr != nil {
			msg := serr.Error()
			_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusFailed, nil, &msg)
			return
		}
		p := im.Prompt
		if p == "" {
			p = prompt
		}
		chPtr := chapterID
		gimg := models.GeneratedImage{
			ID:        imgID,
			JobID:     jobID,
			ChapterID: &chPtr,
			Prompt:    p,
			ImageURL:  url,
			CreatedAt: time.Now().UTC(),
		}
		if err := u.jobs.InsertGeneratedImage(ctx, gimg); err != nil {
			msg := err.Error()
			_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusFailed, nil, &msg)
			return
		}
		apiImages = append(apiImages, imgRow{ID: imgID.String(), ImageURL: url, Prompt: p})
	}

	result := map[string]any{"images": apiImages}
	raw, err := json.Marshal(result)
	if err != nil {
		msg := err.Error()
		_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusFailed, nil, &msg)
		return
	}
	_ = u.jobs.UpdateJob(ctx, jobID, constants.AIJobStatusCompleted, raw, nil)
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
