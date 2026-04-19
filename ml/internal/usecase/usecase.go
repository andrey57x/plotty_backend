package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

type EmbeddingsProvider interface {
	GetEmbedding(ctx context.Context, text string) ([]float32, error)
}

type AIUsecase struct {
	repo         repository.MLRepository
	spellchecker Spellchecker
	llm          LLMProvider
	storage      FileStorage
	embeddings   EmbeddingsProvider
	rmqChan      *amqp.Channel
}

func NewAIUsecase(
	repo repository.MLRepository,
	sp Spellchecker,
	llm LLMProvider,
	st FileStorage,
	emb EmbeddingsProvider,
	rmqChan *amqp.Channel,
) *AIUsecase {
	return &AIUsecase{
		repo:         repo,
		spellchecker: sp,
		llm:          llm,
		storage:      st,
		embeddings:   emb,
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

func sha256Hex(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
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
	case "delete_story_lore":
		storyID, _ := uuid.Parse(task.Metadata["story_id"])
		_ = u.repo.DeleteStoryLore(ctx, storyID)
		return nil
	case "canon_check":
		return u.processCanonCheck(ctx, task)
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

const iterativeLoreSystemPrompt = `Ты — анализатор лора. Твоя задача: прочитать состояние мира (ЛОР) до текущей главы и текст новой главы. 
ОБНОВИ состояние мира. Добавь новые детали, измени состояние существующих персонажей/предметов, если с ними что-то случилось. 
Отвечай СТРОГО в формате JSON.
Структура:
{
  "characters":[{"name": "Имя", "state": "Текущее состояние и факты"}],
  "locations":[{"name": "Название", "state": "Текущее состояние и факты"}],
  "items":[{"name": "Предмет", "state": "Текущее состояние и факты"}]
}`

func (u *AIUsecase) processExtractLore(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	storyID, _ := uuid.Parse(task.Metadata["story_id"])
	chapterID, _ := uuid.Parse(task.Metadata["chapter_id"])
	prevChapterIDStr := task.Metadata["prev_chapter_id"]

	_ = u.repo.CreateTask(ctx, taskID, "extract_lore", "story_id: "+storyID.String())

	contentHash := sha256Hex(task.Payload)
	lastHash, _ := u.repo.GetChapterLoreHash(ctx, chapterID)
	if lastHash == contentHash {
		return u.publishResult(ctx, task, "completed", nil, "")
	}

	var prevLore string = "{}"
	if prevChapterIDStr != "" {
		if prevID, err := uuid.Parse(prevChapterIDStr); err == nil {
			lore, err := u.repo.GetLoreByChapterID(ctx, prevID)
			if err == nil && lore != "" {
				prevLore = lore
			}
		}
	}

	userPrompt := fmt.Sprintf("ЛОР ДО ЭТОЙ ГЛАВЫ:\n%s\n\nТЕКСТ НОВОЙ ГЛАВЫ:\n%s", prevLore, task.Payload)

	llmResponse, err := u.llm.SendChat(iterativeLoreSystemPrompt, userPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	cleanJSONStr := cleanJSON(llmResponse)
	if !json.Valid([]byte(cleanJSONStr)) {
		return u.publishResult(ctx, task, "failed", nil, "gigachat returned invalid JSON")
	}

	if err := u.repo.UpsertChapterLorebook(ctx, chapterID, storyID, contentHash, cleanJSONStr); err != nil {
		return u.publishResult(ctx, task, "failed", nil, "failed to save chapter lore: "+err.Error())
	}

	return u.publishResult(ctx, task, "completed", json.RawMessage(cleanJSONStr), "")
}

const canonSystemPrompt = `Ты — строгий критик вселенной "%s".
ОТВЕЧАЙ СТРОГО ПО ДЕЛУ. БЕЗ МАРКДАУН ФОРМАТИРОВАНИЯ. ТОЛЬКО ОБЫЧНЫЙ ТЕКСТ.
Основывайся ТОЛЬКО на предоставленных фактах из канона.
Если фактов недостаточно для вывода, не выдумывай детали, а укажи только на явные несоответствия с текстом.
Твоя задача: найти противоречия между событиями главы и официальным каноном.

ФАКТЫ ИЗ КАНОНА, которые математически близки к тексту главы:
%s

ТЕКСТ ГЛАВЫ:
%s

Учитывай теги фанфика (AU, OOC и тд): %s. Если есть тег AU (Alternate Universe), будь лояльнее к изменениям мира.
Если текст противоречит канону, укажи это списком. Если противоречий нет, верни фразу: Противоречий с каноном не найдено`

func (u *AIUsecase) processCanonCheck(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	fandomSlug := task.Metadata["fandom_slug"]
	warnings := task.Metadata["warnings"]

	_ = u.repo.CreateTask(ctx, taskID, "canon_check", "fandom: "+fandomSlug)

	// 1. Делаем вектор из текста главы (можно резать на чанки, но для начала возьмем весь текст)
	// Если текст длинный (10к символов), лучше вырезать первые 2000 символов для эмбеддинга,
	// либо сделать суммаризацию, но начнем с простого:
	textForEmbedding := task.Payload
	if len(textForEmbedding) > 2000 {
		textForEmbedding = textForEmbedding[:2000]
	}
	vector, err := u.embeddings.GetEmbedding(ctx, textForEmbedding)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "embeddings error: "+err.Error())
	}

	// 2. Ищем релевантные факты (Топ-7 фактов)
	facts, err := u.repo.SearchCanonFacts(ctx, fandomSlug, vector, 7)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "db search error: "+err.Error())
	}

	// Если канона по фандому в БД нет
	if len(facts) == 0 {
		return u.publishResult(ctx, task, "completed", map[string]string{
			"message": "База знаний для этого фандома пока пуста.",
		}, "")
	}

	factsStr := strings.Join(facts, "\n- ")
	userPrompt := fmt.Sprintf(canonSystemPrompt, fandomSlug, "- "+factsStr, task.Payload, warnings)

	// 3. Отправляем в LLM
	llmResponse, err := u.llm.SendChat("Ты строгий критик. Отвечай только на русском языке.", userPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	cleanResponse := strings.ReplaceAll(llmResponse, "**", "")
	cleanResponse = strings.ReplaceAll(cleanResponse, "*", "")

	return u.publishResult(ctx, task, "completed", map[string]string{
		"message": strings.TrimSpace(cleanResponse),
	}, "")
}

const summarySystemPrompt = `Ты — профессиональный книжный редактор. 
Напиши интригующее описание (аннотацию) для истории на основе текста первой главы. 
Опиши завязку, атмосферу и главного героя. НЕ раскрывай сюжетные повороты, интригу и концовку!
Максимум 3-4 предложения, не больше 180 символов. Отвечай только текстом аннотации.`

func (u *AIUsecase) processGenerateSummary(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	storyID, _ := uuid.Parse(task.Metadata["story_id"])

	_ = u.repo.CreateTask(ctx, taskID, "generate_summary", "story_id: "+storyID.String())

	summary, err := u.llm.SendChat(summarySystemPrompt, "Текст первой главы:\n"+task.Payload)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	summary = strings.TrimSpace(summary)

	if err := u.repo.UpdateSummary(ctx, storyID, summary); err != nil {
		return u.publishResult(ctx, task, "failed", nil, "failed to save summary: "+err.Error())
	}

	return u.publishResult(ctx, task, "completed", map[string]string{"summary": summary}, "")
}

const logicSystemPrompt = `Ты — строгий бета-ридер. Твоя цель — найти логические нестыковки в тексте НОВОЙ главы, основываясь на ЛОРе предыдущих глав.
ОТВЕЧАЙ СТРОГО ПО ДЕЛУ. БЕЗ ВВОДНЫХ СЛОВ. БЕЗ СМАЙЛИКОВ И ЭМОДЗИ (никаких крестиков или галочек). БЕЗ МАРКДАУН ФОРМАТИРОВАНИЯ (без звездочек, жирного текста, решеток). ТОЛЬКО ОБЫЧНЫЙ ТЕКСТ.

ЛОР ПРЕДЫДУЩИХ ГЛАВ:
%s

ТЕКСТ НОВОЙ ГЛАВЫ:
%s

Если есть противоречия, перечисли их простым нумерованным списком с кратким объяснением.
Если противоречий нет, ответь ровно одной фразой (без точки в конце): Логических нестыковок не найдено`

func (u *AIUsecase) processLogicCheck(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	storyID, _ := uuid.Parse(task.Metadata["story_id"])

	_ = u.repo.CreateTask(ctx, taskID, "logic_check", "story_id: "+storyID.String())

	prevIDsStr := task.Metadata["prev_chapter_ids"]
	var prevChapterIDs []uuid.UUID
	if prevIDsStr != "" {
		for _, idStr := range strings.Split(prevIDsStr, ",") {
			if id, err := uuid.Parse(idStr); err == nil {
				prevChapterIDs = append(prevChapterIDs, id)
			}
		}
	}

	currentLore, err := u.repo.GetMergedLore(ctx, prevChapterIDs)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "failed to merge lore: "+err.Error())
	}

	if len(prevChapterIDs) == 0 || currentLore == "{}" || currentLore == "" {
		return u.publishResult(ctx, task, "completed", map[string]string{
			"message": "Для этой главы нет предыдущего опубликованного лора. Не с чем сравнивать.",
		}, "")
	}

	userPrompt := fmt.Sprintf(logicSystemPrompt, currentLore, task.Payload)

	llmResponse, err := u.llm.SendChat("Ты — строгий бета-ридер. Отвечай только на русском языке, без форматирования и эмодзи.", userPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	cleanResponse := strings.ReplaceAll(llmResponse, "**", "")
	cleanResponse = strings.ReplaceAll(cleanResponse, "*", "")

	return u.publishResult(ctx, task, "completed", map[string]string{
		"message": strings.TrimSpace(cleanResponse),
	}, "")
}
