package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/fivecode/plotty/internal/infrastructure/gigachat"
	storage "github.com/fivecode/plotty/internal/infrastructure/minio"
	"github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/fivecode/plotty/ml/internal/models"
	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type AIUsecase struct {
	repo     repository.MLRepository
	gigachat *gigachat.Client
	storage  *storage.MinioStorage
	rmqChan  *amqp.Channel // Добавлено для отправки ответа
}

func NewAIUsecase(repo repository.MLRepository, gc *gigachat.Client, st *storage.MinioStorage, rmqChan *amqp.Channel) *AIUsecase {
	return &AIUsecase{
		repo:     repo,
		gigachat: gc,
		storage:  st,
		rmqChan:  rmqChan,
	}
}

// Хелпер: сохраняет результат в БД и кидает ответ в очередь
func (u *AIUsecase) publishResult(ctx context.Context, taskID uuid.UUID, status string, result any, errStr string) error {
	_ = u.repo.UpdateTaskResult(ctx, taskID, status, result)

	var resultRaw []byte
	if result != nil {
		resultRaw, _ = json.Marshal(result)
	}

	msg := rabbitmq.MLResultMessage{
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

const (
	spellcheckSystemPrompt = `Ты — профессиональный корректор. Найди в тексте орфографические, пунктуационные и грамматические ошибки. 
Ответь ТОЛЬКО в формате JSON, без маркдауна. Формат: {"summary": "текст", "items": [{"fragmentText": "...", "message": "...", "suggestion": "..."}]}`
)

func (u *AIUsecase) ProcessSpellcheck(ctx context.Context, taskID uuid.UUID, payload string) error {
	// Достаем текст из пейлоада (Core шлет нам JSON)
	var input struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal([]byte(payload), &input)
	text := input.Content

	if err := u.repo.CreateTask(ctx, taskID, "spellcheck", text); err != nil {
		return err
	}

	rawResponse, err := u.gigachat.SendChat(spellcheckSystemPrompt, text)
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, err.Error())
	}

	log.Printf("DEBUG: Raw AI response: %s", rawResponse)

	cleanJSON := extractJSON(rawResponse)
	if cleanJSON == "" {
		return u.publishResult(ctx, taskID, "failed", nil, "AI returned non-json response")
	}

	var res models.SpellcheckResult
	if err := json.Unmarshal([]byte(cleanJSON), &res); err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, "invalid json from ai")
	}

	contentLower := strings.ToLower(text)
	for i := range res.Items {
		idx := strings.Index(contentLower, strings.ToLower(res.Items[i].FragmentText))
		if idx != -1 {
			res.Items[i].StartOffset = len([]rune(text[:idx]))
			res.Items[i].EndOffset = res.Items[i].StartOffset + len([]rune(res.Items[i].FragmentText))
		}
	}

	return u.publishResult(ctx, taskID, "completed", res, "")
}

const imagePromptEnhancer = `На основе текста главы и пожелания пользователя, составь детальный промпт для нейросети-художника. Опиши композицию, стиль, освещение, цвета. Ответь ТОЛЬКО текстом промпта, без вводных слов. Ограничься 200 символами.`

func (u *AIUsecase) ProcessImageGen(ctx context.Context, taskID uuid.UUID, payload string) error {
	if err := u.repo.CreateTask(ctx, taskID, "image_gen", payload); err != nil {
		return err
	}

	// Достаем данные из JSON-пейлоада
	var input struct {
		Content string `json:"content"`
		Prompt  string `json:"prompt"`
	}
	_ = json.Unmarshal([]byte(payload), &input)

	// 1. УЛУЧШЕНИЕ ПРОМПТА
	promptInput := fmt.Sprintf("Текст: %s\nПожелание: %s", input.Content, input.Prompt)
	enhancedPrompt, err := u.gigachat.SendChat(imagePromptEnhancer, promptInput)
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, err.Error())
	}

	log.Printf("Сгенерирован промпт для художника: %s", enhancedPrompt)

	// 2. ГЕНЕРАЦИЯ КАРТИНКИ
	fileID, err := u.gigachat.GenerateImage(enhancedPrompt)
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, "image gen failed")
	}

	// 3. Скачиваем картинку
	imgData, err := u.gigachat.DownloadFile(fileID)
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, "download failed")
	}

	// 4. Сохраняем в MinIO
	fileName := fmt.Sprintf("%s.jpg", taskID.String())
	fileURL, err := u.storage.Upload(ctx, fileName, bytes.NewReader(imgData), int64(len(imgData)), "image/jpeg")
	if err != nil {
		return u.publishResult(ctx, taskID, "failed", nil, "minio upload failed")
	}

	result := models.ImageResult{
		URL:    fileURL,
		Prompt: enhancedPrompt,
	}

	return u.publishResult(ctx, taskID, "completed", result, "")
}

func extractJSON(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return raw[start : end+1]
}
