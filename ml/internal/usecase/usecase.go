package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

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

func (u *AIUsecase) publishResult(ctx context.Context, task sharedrmq.MLTaskMessage, status string, result any, errStr string) error {
	taskID, _ := uuid.Parse(task.TaskID)
	_ = u.repo.UpdateTaskResult(ctx, taskID, status, result)

	var resultRaw []byte
	if result != nil {
		resultRaw, _ = json.Marshal(result)
	}

	var mlErr *sharedrmq.MLErrorDetails
	if errStr != "" {
		mlErr = &sharedrmq.MLErrorDetails{
			Code:    "ML_INTERNAL_ERROR",
			Message: errStr,
		}
	}

	msg := sharedrmq.MLResultMessage{
		TaskID:   task.TaskID,
		TraceID:  task.TraceID,
		Type:     task.Type,
		Status:   status,
		Result:   resultRaw,
		Error:    mlErr,
		Metadata: task.Metadata,
	}

	body, _ := json.Marshal(msg)
	return u.rmqChan.PublishWithContext(ctx, "", "ml_results_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}

func (u *AIUsecase) ProcessSpellcheck(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	var input struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal([]byte(task.Payload), &input)

	if err := u.repo.CreateTask(ctx, taskID, "spellcheck", input.Content); err != nil {
		return err
	}

	res, err := u.spellchecker.CheckText(ctx, input.Content)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, err.Error())
	}

	return u.publishResult(ctx, task, "completed", res, "")
}

func (u *AIUsecase) ProcessMLTask(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	switch task.Type {
	case "image_gen":
		return u.processImageGen(ctx, task)
	case "extract_lore":
		return u.processExtractLore(ctx, task)
	case "generate_summary":
		return u.processGenerateSummary(ctx, task)
	case "logic_check":
		return u.processLogicCheck(ctx, task)
	default:
		return fmt.Errorf("unknown ml task type: %s", task.Type)
	}
}

const imagePromptEnhancer = `На основе текста главы и пожелания пользователя, составь детальный промпт для нейросети-художника. Опиши композицию, стиль, освещение, цвета. Ответь ТОЛЬКО текстом промпта, без вводных слов. Ограничься 200 символами.`

func (u *AIUsecase) processImageGen(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	if err := u.repo.CreateTask(ctx, taskID, "image_gen", task.Payload); err != nil {
		return err
	}

	var input struct {
		Content string `json:"content"`
		Prompt  string `json:"prompt"`
	}
	_ = json.Unmarshal([]byte(task.Payload), &input)

	promptInput := fmt.Sprintf("Текст: %s\nПожелание: %s", input.Content, input.Prompt)
	enhancedPrompt, err := u.llm.SendChat(imagePromptEnhancer, promptInput)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, err.Error())
	}

	fileID, err := u.llm.GenerateImage(enhancedPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "image gen failed")
	}

	imgData, err := u.llm.DownloadFile(fileID)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "download failed")
	}

	fileName := fmt.Sprintf("%s.jpg", taskID.String())
	fileURL, err := u.storage.Upload(ctx, fileName, bytes.NewReader(imgData), int64(len(imgData)), "image/jpeg")
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "minio upload failed")
	}

	return u.publishResult(ctx, task, "completed", models.ImageResult{URL: fileURL, Prompt: enhancedPrompt}, "")
}

func cleanJSON(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```json") {
		input = strings.TrimPrefix(input, "```json")
	} else if strings.HasPrefix(input, "```") {
		input = strings.TrimPrefix(input, "```")
	}
	input = strings.TrimSuffix(input, "```")
	return strings.TrimSpace(input)
}

const loreSystemPrompt = `Ты — аналитик сюжета. Анализируй текст и извлекай факты.
У тебя есть "Текущий лор" (в формате JSON) и "Текст новой главы".
Твоя задача: ДОПОЛНИТЬ текущий лор.
1. Добавь новых персонажей/предметы/локации.
2. Измени состояние существующих, если в тексте что-то произошло.
3. ВАЖНО: НИКОГДА НЕ УДАЛЯЙ персонажей, локации и предметы из старого лора, даже если они не упоминаются в новой главе! Просто оставь их как есть.
Верни результат СТРОГО в формате JSON следующей структуры:
{
  "characters":[{"name": "Имя", "state": "Текущее состояние"}],
  "locations":[{"name": "Название", "state": "Описание"}],
  "items":[{"name": "Предмет", "state": "Состояние"}]
}
Не пиши ничего кроме JSON-объекта!`

func (u *AIUsecase) processExtractLore(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	storyID, _ := uuid.Parse(task.Metadata["story_id"])
	chapterID, _ := uuid.Parse(task.Metadata["chapter_id"])

	_ = u.repo.CreateTask(ctx, taskID, "extract_lore", "story_id: "+storyID.String())

	// 1. Получаем текущий лор
	currentLore, err := u.repo.GetLorebook(ctx, storyID)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "failed to get current lore: "+err.Error())
	}

	// 2. Формируем запрос
	userPrompt := fmt.Sprintf("Текущий лор: %s\n\nТекст новой главы:\n%s", currentLore, task.Payload)

	// 3. Отправляем в GigaChat
	llmResponse, err := u.llm.SendChat(loreSystemPrompt, userPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	// 4. Очищаем JSON и проверяем валидность
	cleanJSONStr := cleanJSON(llmResponse)
	if !json.Valid([]byte(cleanJSONStr)) {
		return u.publishResult(ctx, task, "failed", nil, "gigachat returned invalid JSON: "+cleanJSONStr)
	}

	// 5. Сохраняем в БД ML
	if err := u.repo.UpsertLorebook(ctx, storyID, chapterID, cleanJSONStr); err != nil {
		return u.publishResult(ctx, task, "failed", nil, "failed to save lore: "+err.Error())
	}

	return u.publishResult(ctx, task, "completed", json.RawMessage(cleanJSONStr), "")
}

const summarySystemPrompt = `Ты — профессиональный книжный редактор. 
Напиши интригующее описание (аннотацию) для истории на основе текста первой главы. 
Опиши завязку, атмосферу и главного героя. НЕ раскрывай сюжетные повороты, интригу и концовку!
Максимум 3-4 предложения. Отвечай только текстом аннотации.`

func (u *AIUsecase) processGenerateSummary(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	storyID, _ := uuid.Parse(task.Metadata["story_id"])

	_ = u.repo.CreateTask(ctx, taskID, "generate_summary", "story_id: "+storyID.String())

	// Отправляем текст 1-ой главы в GigaChat
	summary, err := u.llm.SendChat(summarySystemPrompt, "Текст первой главы:\n"+task.Payload)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	// Очищаем от возможных кавычек
	summary = strings.TrimSpace(summary)

	// Сохраняем Summary в БД
	if err := u.repo.UpdateSummary(ctx, storyID, summary); err != nil {
		return u.publishResult(ctx, task, "failed", nil, "failed to save summary: "+err.Error())
	}

	return u.publishResult(ctx, task, "completed", map[string]string{"summary": summary}, "")
}

const logicSystemPrompt = `Ты — строгий бета-ридер. Твоя цель — найти логические нестыковки в новом тексте главы, основываясь на лоре (базе знаний) предыдущих глав.
ЛОР:
%s

ТЕКСТ НОВОЙ ГЛАВЫ:
%s

Если есть противоречия (например, используется сломанный предмет, персонаж находится в двух местах одновременно, воскрес мертвый и т.д.), перечисли их списком с объяснением, почему это нелогично.
Если противоречий нет, ответь коротко: "Логических нестыковок не найдено."`

func (u *AIUsecase) processLogicCheck(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	storyID, _ := uuid.Parse(task.Metadata["story_id"])

	_ = u.repo.CreateTask(ctx, taskID, "logic_check", "story_id: "+storyID.String())

	// 1. Получаем текущий лор
	currentLore, err := u.repo.GetLorebook(ctx, storyID)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "failed to get current lore: "+err.Error())
	}

	// Если лор пустой (история еще ни разу не публиковалась)
	if currentLore == "{}" || currentLore == "" {
		return u.publishResult(ctx, task, "completed", map[string]string{
			"message": "База знаний еще пуста (нет опубликованных глав). Логику пока проверять не на чем.",
		}, "")
	}

	// 2. Формируем промпт
	userPrompt := fmt.Sprintf(logicSystemPrompt, currentLore, task.Payload)

	// 3. Отправляем в GigaChat
	llmResponse, err := u.llm.SendChat("Ты — строгий бета-ридер. Отвечай только на русском языке.", userPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	// 4. Возвращаем результат
	return u.publishResult(ctx, task, "completed", map[string]string{
		"message": strings.TrimSpace(llmResponse),
	}, "")
}
