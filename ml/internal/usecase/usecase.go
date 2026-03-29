package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/fivecode/plotty/ml/internal/models"
	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)


type Spellchecker interface {
	CheckText(ctx context.Context, text string) (models.SpellcheckResult, error)
}

type LLMProvider interface {
	SendChat(systemPrompt, userText string) (string, error)
	GenerateImage(prompt string) (string, error)
	DownloadFile(fileID string) ([]byte, error)
}

type FileStorage interface {
	Upload(ctx context.Context, fileName string, reader io.Reader, size int64, contentType string) (string, error)
}


type AIUsecase struct {
	repo         repository.MLRepository
	spellchecker Spellchecker
	llm          LLMProvider
	storage      FileStorage
	rmqChan      *amqp.Channel
}

func NewAIUsecase(repo repository.MLRepository, sp Spellchecker, llm LLMProvider, st FileStorage, rmqChan *amqp.Channel) *AIUsecase {
	return &AIUsecase{
		repo:         repo,
		spellchecker: sp,
		llm:          llm,
		storage:      st,
		rmqChan:      rmqChan,
	}
}

func (u *AIUsecase) publishResult(ctx context.Context, taskID uuid.UUID, status string, result any, errStr string) error {
	_ = u.repo.UpdateTaskResult(ctx, taskID, status, result)

	var resultRaw []byte
	if result != nil {
		resultRaw, _ = json.Marshal(result)
	}

	msg := sharedrmq.MLResultMessage{
		TaskID: taskID.String(),
		Status: status,
		Result: resultRaw,
		Error:  errStr,
	}

	body, _ := json.Marshal(msg)
	return u.rmqChan.PublishWithContext(ctx, "", "ml_results_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}


func (u *AIUsecase) ProcessSpellcheck(ctx context.Context, taskID uuid.UUID, payload string) error {
	var input struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal([]byte(payload), &input)

	if err := u.repo.CreateTask(ctx, taskID, "spellcheck", input.Content); err != nil {
		return err
	}

	res, err := u.spellchecker.CheckText(ctx, input.Content)
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, err.Error())
	}

	return u.publishResult(ctx, taskID, "completed", res, "")
}


func (u *AIUsecase) ProcessMLTask(ctx context.Context, taskID uuid.UUID, taskType, payload string) error {
	switch taskType {
	case "image_gen":
		return u.processImageGen(ctx, taskID, payload)
	default:
		return fmt.Errorf("unknown ml task type: %s", taskType)
	}
}

const imagePromptEnhancer = `На основе текста главы и пожелания пользователя, составь детальный промпт для нейросети-художника. Опиши композицию, стиль, освещение, цвета. Ответь ТОЛЬКО текстом промпта, без вводных слов. Ограничься 200 символами.`

func (u *AIUsecase) processImageGen(ctx context.Context, taskID uuid.UUID, payload string) error {
	if err := u.repo.CreateTask(ctx, taskID, "image_gen", payload); err != nil {
		return err
	}

	var input struct {
		Content string `json:"content"`
		Prompt  string `json:"prompt"`
	}
	_ = json.Unmarshal([]byte(payload), &input)

	promptInput := fmt.Sprintf("Текст: %s\nПожелание: %s", input.Content, input.Prompt)
	enhancedPrompt, err := u.llm.SendChat(imagePromptEnhancer, promptInput)
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, err.Error())
	}

	fileID, err := u.llm.GenerateImage(enhancedPrompt)
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, "image gen failed")
	}

	imgData, err := u.llm.DownloadFile(fileID)
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, "download failed")
	}

	fileName := fmt.Sprintf("%s.jpg", taskID.String())
	fileURL, err := u.storage.Upload(ctx, fileName, bytes.NewReader(imgData), int64(len(imgData)), "image/jpeg")
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, "minio upload failed")
	}

	return u.publishResult(ctx, taskID, "completed", models.ImageResult{URL: fileURL, Prompt: enhancedPrompt}, "")
}
