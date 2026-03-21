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
	"github.com/fivecode/plotty/ml/internal/models"
	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/google/uuid"
)

type AIUsecase struct {
	repo     repository.MLRepository
	gigachat *gigachat.Client
	storage  *storage.MinioStorage
}

func NewAIUsecase(repo repository.MLRepository, gc *gigachat.Client, st *storage.MinioStorage) *AIUsecase {
	return &AIUsecase{
		repo:     repo,
		gigachat: gc,
		storage:  st,
	}
}

const (
	spellcheckSystemPrompt = `Ты — профессиональный корректор. Найди в тексте орфографические, пунктуационные и грамматические ошибки. 
Ответь ТОЛЬКО в формате JSON, без маркдауна. Формат: {"summary": "текст", "items": [{"fragmentText": "...", "message": "...", "suggestion": "..."}]}`
)

func (u *AIUsecase) ProcessSpellcheck(ctx context.Context, taskID uuid.UUID, text string) error {
	// Создаем запись в нашей БД
	if err := u.repo.CreateTask(ctx, taskID, "spellcheck", text); err != nil {
		return err
	}

	rawResponse, err := u.gigachat.SendChat(spellcheckSystemPrompt, text)
	if err != nil {
		u.repo.UpdateTaskResult(ctx, taskID, "failed", nil)
		return err
	}

	// ДОБАВЬ ЭТУ СТРОЧКУ, ЧТОБЫ УВИДЕТЬ, ЧТО ИМЕННО ПРИСЛАЛ ИИ
	log.Printf("DEBUG: Raw AI response: %s", rawResponse)

	// ИСПОЛЬЗУЕМ НОВУЮ ФУНКЦИЮ ДЛЯ ОЧИСТКИ
	cleanJSON := extractJSON(rawResponse)
	if cleanJSON == "" {
		u.repo.UpdateTaskResult(ctx, taskID, "failed", map[string]string{"error": "AI returned non-json response"})
		return fmt.Errorf("ИИ вернул не JSON ответ: %s", rawResponse)
	}

	var res models.SpellcheckResult
	if err := json.Unmarshal([]byte(cleanJSON), &res); err != nil {
		u.repo.UpdateTaskResult(ctx, taskID, "failed", map[string]string{"error": "invalid json from ai"})
		return fmt.Errorf("невалидный JSON от ИИ: %w", err)
	}

	contentLower := strings.ToLower(text)
	for i := range res.Items {
		idx := strings.Index(contentLower, strings.ToLower(res.Items[i].FragmentText))
		if idx != -1 {
			res.Items[i].StartOffset = len([]rune(text[:idx]))
			res.Items[i].EndOffset = res.Items[i].StartOffset + len([]rune(res.Items[i].FragmentText))
		}
	}

	return u.repo.UpdateTaskResult(ctx, taskID, "completed", res)
}

const imagePromptEnhancer = `На основе текста главы и пожелания пользователя, составь детальный промпт для нейросети-художника. Опиши композицию, стиль, освещение, цвета. Ответь ТОЛЬКО текстом промпта, без вводных слов. Ограничься 200 символами.`

func (u *AIUsecase) ProcessImageGen(ctx context.Context, taskID uuid.UUID, payload string) error {
	// 0. Разбиваем payload
	parts := strings.Split(payload, "|")
	chapterText := parts[0]
	userWish := ""
	if len(parts) > 1 {
		userWish = parts[1]
	}

	if err := u.repo.CreateTask(ctx, taskID, "image_gen", payload); err != nil {
		return err
	}

	// 1. УЛУЧШЕНИЕ ПРОМПТА (Вызываем как обычный чат)
	promptInput := fmt.Sprintf("Текст: %s\nПожелание: %s", chapterText, userWish)
	enhancedPrompt, err := u.gigachat.SendChat(imagePromptEnhancer, promptInput)
	if err != nil {
		u.repo.UpdateTaskResult(ctx, taskID, "failed", nil)
		return err
	}

	log.Printf("Сгенерирован промпт для художника: %s", enhancedPrompt)

	// 2. ГЕНЕРАЦИЯ КАРТИНКИ (Вызывает text2image через function_call: "auto")
	fileID, err := u.gigachat.GenerateImage(enhancedPrompt)
	if err != nil {
		u.repo.UpdateTaskResult(ctx, taskID, "failed", map[string]string{"error": "image gen failed"})
		return err
	}

	// 3. Скачиваем картинку
	imgData, err := u.gigachat.DownloadFile(fileID)
	if err != nil {
		u.repo.UpdateTaskResult(ctx, taskID, "failed", map[string]string{"error": "download failed"})
		return err
	}

	// 4. Сохраняем в MinIO
	fileName := fmt.Sprintf("%s.jpg", taskID.String()) // Сбер отдает в JPG
	fileURL, err := u.storage.Upload(ctx, fileName, bytes.NewReader(imgData), int64(len(imgData)), "image/jpeg")
	if err != nil {
		u.repo.UpdateTaskResult(ctx, taskID, "failed", map[string]string{"error": "minio upload failed"})
		return err
	}

	result := models.ImageResult{
		URL:    fileURL,
		Prompt: enhancedPrompt, // Сохраняем промпт, чтобы юзер видел, как ИИ его понял
	}

	return u.repo.UpdateTaskResult(ctx, taskID, "completed", result)
}

// extractJSON находит и извлекает JSON объект из строки, даже если он окружен мусором
func extractJSON(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start == -1 || end == -1 || end < start {
		return "" // Не удалось найти валидный JSON
	}
	return raw[start : end+1]
}
