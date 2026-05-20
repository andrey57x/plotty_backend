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

	"github.com/fivecode/plotty/internal/infrastructure/gigachat"
	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/fivecode/plotty/ml/internal/models"
	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Spellchecker interface {
	CheckText(ctx context.Context, text string, allowedWords []string) (models.SpellcheckResult, error)
}

type LLMProvider interface {
	SendChat(modelName, systemPrompt, userText string) (string, error)
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
	storyIDStr := task.Metadata["story_id"]
	fandomSlug := task.Metadata["fandom_slug"]

	var input struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal([]byte(task.Payload), &input)

	if err := u.repo.CreateTask(ctx, taskID, "spellcheck", input.Content); err != nil {
		return err
	}

	allowedWords := make([]string, 0)
	if storyID, err := uuid.Parse(storyIDStr); err == nil && storyID != uuid.Nil {
		storyNames, _ := u.repo.GetStoryLoreNames(ctx, storyID)
		allowedWords = append(allowedWords, storyNames...)
	}
	if fandomSlug != "" {
		canonNames, _ := u.repo.GetCanonEntityNames(ctx, fandomSlug)
		allowedWords = append(allowedWords, canonNames...)
	}

	res, err := u.spellchecker.CheckText(ctx, input.Content, allowedWords)
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
	case "generate_fandom_lore":
		return u.processGenerateFandomLore(ctx, task)
	default:
		return fmt.Errorf("unknown ml task type: %s", task.Type)
	}
}

const imagePromptEnhancer = `На основе текста главы и пожелания пользователя, составь детальный промпт для нейросети-художника. Основой выступает текст главы, а пожелания пользователя уточняют. Опиши композицию, стиль, освещение, цвета. Ответь ТОЛЬКО текстом промпта, без вводных слов. Ограничься 200 символами.`

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
	enhancedPrompt, err := u.llm.SendChat(gigachat.ModelGigaChat, imagePromptEnhancer, promptInput)
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

	llmResponse, err := u.llm.SendChat(gigachat.ModelGigaChat, iterativeLoreSystemPrompt, userPrompt)
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

func chunkText(text string, maxLen int) []string {
	paragraphs := strings.Split(text, "\n")
	var chunks []string
	var current string

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(current)+len(p) > maxLen && len(current) > 0 {
			chunks = append(chunks, current)
			current = p
		} else {
			if current == "" {
				current = p
			} else {
				current += "\n" + p
			}
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}

const canonSystemPrompt = `Ты — строгий критик вселенной "%s".
ОТВЕЧАЙ СТРОГО ПО ДЕЛУ. БЕЗ МАРКДАУН ФОРМАТИРОВАНИЯ. ТОЛЬКО ОБЫЧНЫЙ ТЕКСТ.
Основывайся ТОЛЬКО на предоставленных фактах из канона.
Если фактов недостаточно для вывода, не выдумывай детали, а укажи только на явные несоответствия с текстом.
Твоя задача: найти противоречия между событиями главы и официальным каноном.

ФАКТЫ ИЗ КАНОНА, которые были найдены для разных сцен главы:
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

	// 1. Бьем текст главы на чанки
	chunks := chunkText(task.Payload, 1000)

	// Используем map для удаления дубликатов фактов
	factSet := make(map[string]struct{})
	var uniqueFacts []string

	// 2. Ищем факты для КАЖДОГО чанка
	for _, chunk := range chunks {
		vector, err := u.embeddings.GetEmbedding(ctx, chunk)
		if err != nil {
			continue // Если один чанк не сработал, идем дальше
		}

		// Берем топ-2 факта для каждого абзаца
		facts, err := u.repo.SearchCanonFacts(ctx, fandomSlug, vector, 2)
		if err != nil {
			continue
		}

		for _, f := range facts {
			if _, exists := factSet[f]; !exists {
				factSet[f] = struct{}{}
				uniqueFacts = append(uniqueFacts, f)
			}
		}
	}

	if len(uniqueFacts) == 0 {
		return u.publishResult(ctx, task, "completed", map[string]string{
			"message": "База знаний для этого фандома пока пуста или не удалось найти подходящие факты.",
		}, "")
	}

	factsStr := strings.Join(uniqueFacts, "\n- ")
	userPrompt := fmt.Sprintf(canonSystemPrompt, fandomSlug, "- "+factsStr, task.Payload, warnings)

	// 3. Отправляем всё это богатство в LLM
	llmResponse, err := u.llm.SendChat(gigachat.ModelGigaChat, "Ты строгий критик. Отвечай только на русском языке.", userPrompt)
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

	summary, err := u.llm.SendChat(gigachat.ModelGigaChat, summarySystemPrompt, "Текст первой главы:\n"+task.Payload)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}
	summary = strings.TrimSpace(summary)

	embedding, err := u.embeddings.GetEmbedding(ctx, summary)
	if err != nil {
		_ = u.repo.UpdateSummary(ctx, storyID, summary)
		return u.publishResult(ctx, task, "failed", nil, "embedding error: "+err.Error())
	}

	if err := u.repo.UpdateSummaryAndEmbedding(ctx, storyID, summary, embedding); err != nil {
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

	llmResponse, err := u.llm.SendChat(gigachat.ModelGigaChat, "Ты — строгий бета-ридер. Отвечай только на русском языке, без форматирования и эмодзи.", userPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	cleanResponse := strings.ReplaceAll(llmResponse, "**", "")
	cleanResponse = strings.ReplaceAll(cleanResponse, "*", "")

	return u.publishResult(ctx, task, "completed", map[string]string{
		"message": strings.TrimSpace(cleanResponse),
	}, "")
}

const fandomLoreSystemPrompt = `Ты — эксперт-архивариус вымышленных миров. Твоя задача: на основе пользовательского описания вселенной создать структурированную базу знаний (ЛОР).
Выдели до 30 ключевых фактов: персонажи, законы физики/магии, артефакты, локации.
ОТВЕЧАЙ СТРОГО В ФОРМАТЕ JSON-МАССИВА, БЕЗ МАРКДАУН РАЗМЕТКИ (слова без json), БЕЗ ВВОДНЫХ И СЛЕДУЮЩИХ СЛОВ.
Формат ответа:
[
  {"entity": "Имя или Название", "fact": "Подробное описание факта или правила"}
]`

func (u *AIUsecase) processGenerateFandomLore(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	fandomSlug := task.Metadata["fandom_slug"]

	_ = u.repo.CreateTask(ctx, taskID, "generate_fandom_lore", "fandom_slug: "+fandomSlug)

	// 1. Просим GigaChat-Pro сгенерировать JSON массив фактов
	llmResponse, err := u.llm.SendChat(gigachat.ModelGigaChatPro, fandomLoreSystemPrompt, "Описание вселенной:\n"+task.Payload)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	// 2. Очищаем ответ от возможного мусора (маркдауна)
	cleanJSONStr := cleanJSON(llmResponse)
	
	var rawFacts []map[string]string
	if err := json.Unmarshal([]byte(cleanJSONStr), &rawFacts); err != nil {
		return u.publishResult(ctx, task, "failed", nil, "invalid json from llm: "+err.Error())
	}

	// 3. Проходим по каждому факту, генерируем вектор и сохраняем
	inserted := 0
	for _, item := range rawFacts {
		entity := item["entity"]
		fact := item["fact"]

		if entity == "" || fact == "" {
			continue
		}

		// Делаем эмбеддинг факта
		vector, err := u.embeddings.GetEmbedding(ctx, fact)
		if err != nil {
			continue // если один упал, пропускаем, идем дальше
		}

		canonFact := models.CanonFact{
			ID:         uuid.New(),
			FandomSlug: fandomSlug,
			EntityName: entity,
			FactText:   fact,
			Embedding:  vector,
		}

		// Сохраняем в БД
		if err := u.repo.InsertCanonFact(ctx, canonFact); err == nil {
			inserted++
		}
	}

	resultMsg := map[string]any{
		"message":        "Фэндом успешно обработан",
		"facts_inserted": inserted,
	}

	return u.publishResult(ctx, task, "completed", resultMsg, "")
}
