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
	"time"

	"github.com/fivecode/plotty/internal/infrastructure/gigachat"
	sharedrmq "github.com/fivecode/plotty/internal/infrastructure/rabbitmq"
	"github.com/fivecode/plotty/ml/internal/models"
	"github.com/fivecode/plotty/ml/internal/repository"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog/log"
)

type Spellchecker interface {
	CheckText(ctx context.Context, text string, allowedStems map[string]struct{}) (models.SpellcheckResult, error)
}

type LLMProvider interface {
	SendChat(modelName, systemPrompt, userText string) (string, gigachat.Usage, error)
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
	vocabManager *VocabularyManager // НАШ НОВЫЙ МЕНЕДЖЕР СЛОВАРЕЙ В RAM
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
		vocabManager: NewVocabularyManager(repo), // Инициализация менеджера
	}
}

// publishResultWithTokens логирует токены и отправляет результат
func (u *AIUsecase) publishResultWithTokens(ctx context.Context, task sharedrmq.MLTaskMessage, status string, result any, errStr string, promptTokens, completionTokens, totalTokens int) error {
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
		TaskID:           task.TaskID,
		TraceID:          task.TraceID,
		Type:             task.Type,
		Status:           status,
		Result:           resultRaw,
		Error:            mlErr,
		Metadata:         task.Metadata,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}

	// Точные логи по использованию токенов в stdout воркера
	log.Info().
		Str("trace_id", task.TraceID).
		Str("task_id", task.TaskID).
		Str("task_type", task.Type).
		Int("prompt_tokens", promptTokens).
		Int("completion_tokens", completionTokens).
		Int("total_tokens", totalTokens).
		Msg("Токены GigaChat для этой задачи успешно логированы")

	body, _ := json.Marshal(msg)
	return u.rmqChan.PublishWithContext(ctx, "", "ml_results_queue", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}

func (u *AIUsecase) publishResult(ctx context.Context, task sharedrmq.MLTaskMessage, status string, result any, errStr string) error {
	return u.publishResultWithTokens(ctx, task, status, result, errStr, 0, 0, 0)
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

	// Собираем основы разрешенных слов (Fandom + Story)
	allowedStems := make(map[string]struct{})

	// 1. Подгружаем основы слов канона фэндома
	if fandomSlug != "" {
		fandomVocab := u.vocabManager.GetFandomVocabulary(ctx, fandomSlug)
		for k := range fandomVocab {
			allowedStems[k] = struct{}{}
		}
	}

	// 2. Подгружаем основы слов, созданные автором конкретного фанфика
	if storyID, err := uuid.Parse(storyIDStr); err == nil && storyID != uuid.Nil {
		storyVocab := u.vocabManager.GetStoryVocabulary(ctx, storyID)
		for k := range storyVocab {
			allowedStems[k] = struct{}{}
		}
	}

	// 3. Вызываем проверку орфографии, передавая карту разрешенных основ
	res, err := u.spellchecker.CheckText(ctx, input.Content, allowedStems)
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
	log.Info().
		Str("trace_id", task.TraceID).
		Str("task_id", task.TaskID).
		Msg("Начало обработки генерации изображения")

	if err := u.repo.CreateTask(ctx, taskID, "image_gen", task.Payload); err != nil {
		log.Error().
			Err(err).
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Msg("Ошибка создания задачи в базе")
		return err
	}

	var input struct {
		Content string `json:"content"`
		Prompt  string `json:"prompt"`
	}
	if err := json.Unmarshal([]byte(task.Payload), &input); err != nil {
		log.Error().
			Err(err).
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Msg("Ошибка десериализации payload")
		return u.publishResult(ctx, task, "failed", nil, "invalid payload format")
	}

	promptInput := fmt.Sprintf("Текст: %s\nПожелание: %s", input.Content, input.Prompt)
	log.Info().
		Str("trace_id", task.TraceID).
		Str("task_id", task.TaskID).
		Msg("Шаг 1: Улучшение промпта через GigaChat...")

	enhancedPrompt, usage, err := u.llm.SendChat(gigachat.ModelGigaChat, imagePromptEnhancer, promptInput)
	if err != nil {
		log.Error().
			Err(err).
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Msg("Ошибка улучшения промпта")
		return u.publishResult(ctx, task, "failed", nil, fmt.Sprintf("prompt enhancement failed: %v", err))
	}

	log.Info().
		Str("trace_id", task.TraceID).
		Str("task_id", task.TaskID).
		Str("enhanced_prompt", enhancedPrompt).
		Msg("Промпт улучшен")

	var fileID string
	var imgData []byte
	const maxAttempts = 3
	var attemptErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Info().
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Int("attempt", attempt).
			Int("max_attempts", maxAttempts).
			Msg("Попытка генерации изображения")

		fileID, attemptErr = u.llm.GenerateImage(enhancedPrompt)
		if attemptErr != nil {
			log.Error().
				Err(attemptErr).
				Str("trace_id", task.TraceID).
				Str("task_id", task.TaskID).
				Int("attempt", attempt).
				Msg("Ошибка генерации изображения на попытке")
			if attempt < maxAttempts {
				time.Sleep(2 * time.Second)
			}
			continue
		}

		log.Info().
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Int("attempt", attempt).
			Str("file_id", fileID).
			Msg("Изображение сгенерировано, скачивание...")

		imgData, attemptErr = u.llm.DownloadFile(fileID)
		if attemptErr != nil {
			log.Error().
				Err(attemptErr).
				Str("trace_id", task.TraceID).
				Str("task_id", task.TaskID).
				Int("attempt", attempt).
				Msg("Ошибка скачивания файла на попытке")
			if attempt < maxAttempts {
				time.Sleep(2 * time.Second)
			}
			continue
		}

		attemptErr = nil
		break
	}

	if attemptErr != nil {
		log.Error().
			Err(attemptErr).
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Int("max_attempts", maxAttempts).
			Msg("Все попытки генерации провалились")
		return u.publishResultWithTokens(ctx, task, "failed", nil, fmt.Sprintf("image generation failed after %d attempts: %v", maxAttempts, attemptErr), usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	log.Info().
		Str("trace_id", task.TraceID).
		Str("task_id", task.TaskID).
		Msg("Шаг 4: Загрузка изображения в S3 (MinIO)...")

	fileName := fmt.Sprintf("%s.jpg", taskID.String())
	fileURL, err := u.storage.Upload(ctx, fileName, bytes.NewReader(imgData), int64(len(imgData)), "image/jpeg")
	if err != nil {
		log.Error().
			Err(err).
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Msg("Ошибка загрузки в MinIO")
		return u.publishResultWithTokens(ctx, task, "failed", nil, fmt.Sprintf("minio upload failed: %v", err), usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	log.Info().
		Str("trace_id", task.TraceID).
		Str("task_id", task.TaskID).
		Str("file_url", fileURL).
		Msg("Изображение успешно сгенерировано и загружено")

	return u.publishResultWithTokens(ctx, task, "completed", models.ImageResult{URL: fileURL, Prompt: enhancedPrompt}, "", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
}

func findMentionedEntities(text string, entityNames []string) []string {
	textLower := strings.ToLower(text)
	var matched []string
	seen := make(map[string]bool)

	for _, name := range entityNames {
		nameLower := strings.ToLower(name)
		words := strings.Fields(nameLower)
		if len(words) == 0 {
			continue
		}

		hasMatch := false
		for _, w := range words {
			w = strings.Trim(w, ".,!?-()\"'«» \t")
			if len([]rune(w)) < 3 {
				continue
			}

			stem := w
			suffixes := []string{"ом", "ем", "ой", "ей", "ая", "ое", "ый", "ий", "ых", "их", "а", "я", "о", "е", "ы", "и", "у"}
			for _, suff := range suffixes {
				if strings.HasSuffix(w, suff) && len([]rune(w)) > len([]rune(suff))+2 {
					stem = w[:len(w)-len(suff)]
					break
				}
			}

			if strings.Contains(textLower, stem) {
				hasMatch = true
				break
			}
		}

		if hasMatch && !seen[nameLower] {
			seen[nameLower] = true
			matched = append(matched, nameLower) // приведение в нижний регистр для запроса ANY() в PostgreSQL
		}
	}
	return matched
}

const canonSystemPrompt = `Ты — строгий критик вселенной "%s".
ОТВЕЧАЙ СТРОГО ПО ДЕЛУ. БЕЗ МАРКДАУН ФОРМАТИРОВАНИЯ. ТОЛЬКО ОБЫЧНЫЙ ТЕКСТ.

ВНИМАНИЕ: Тебе предоставлены различные факты канона. Некоторые из них могут быть совершенно не связаны с текущим текстом главы. Игнорируй те факты, которые не упоминаются и не имеют никакого отношения к происходящему в тексте главы. Не пытайся притянуть их за уши ради выполнения задачи!

Указывай ТОЛЬКО те противоречия, которые реально происходят в присланном тексте главы. Не пиши о правилах из предоставленного контекста, которые НЕ были нарушены в данном отрывке. Не давай автору советов на будущее и не комментируй правила, если они не нарушены.

Тебе нужно написать ТОЛЬКО список противоречий, либо фразу: Противоречий с каноном не найдено

Тебе ЗАПРЕЩЕНО писать что-либо, кроме списка противоречий.

Опирайся в первую очередь на предоставленные факты из канона. Если предоставленных фактов недостаточно для вывода, ты имеешь право использовать свои собственные глубокие знания об этой вселенной для выявления явных, грубых и очевидных противоречий с каноном. 

При этом будь лоялен к мелкому художественному вымыслу и не придумывай несуществующие детали сюжета.

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

	log.Info().
		Str("trace_id", task.TraceID).
		Str("task_id", task.TaskID).
		Str("fandom", fandomSlug).
		Msg("Начало обработки проверки на канон (Entity Matcher)")

	_ = u.repo.CreateTask(ctx, taskID, "canon_check", "fandom: "+fandomSlug)

	// 1. Получаем список всех известных каноничных имен фэндома
	entityNames, err := u.repo.GetCanonEntityNames(ctx, fandomSlug)
	if err != nil {
		log.Error().
			Err(err).
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Str("fandom", fandomSlug).
			Msg("Ошибка получения списка имен сущностей")
	}

	// 2. Ищем упомянутые сущности в тексте черновика
	mentioned := findMentionedEntities(task.Payload, entityNames)

	var uniqueFacts []string

	// 3. Вытаскиваем глобальные правила мира
	globalFacts, err := u.repo.GetGlobalCanonFacts(ctx, fandomSlug)
	if err == nil && len(globalFacts) > 0 {
		uniqueFacts = append(uniqueFacts, globalFacts...)
	}

	// 4. Вытаскиваем специфические факты по найденным персонажам
	if len(mentioned) > 0 {
		entityFacts, err := u.repo.GetCanonFactsByEntities(ctx, fandomSlug, mentioned)
		if err == nil && len(entityFacts) > 0 {
			uniqueFacts = append(uniqueFacts, entityFacts...)
		}
	}

	if len(uniqueFacts) == 0 {
		log.Warn().
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Msg("База знаний пуста или факты не найдены")
		return u.publishResult(ctx, task, "completed", map[string]string{
			"message": "База знаний для этого фандома пока пуста или не удалось найти подходящие факты.",
		}, "")
	}

	// Жесткий лимит до 20 фактов, как вы и просили для оптимизации токенов
	if len(uniqueFacts) > 20 {
		uniqueFacts = uniqueFacts[:20]
	}

	factsStr := strings.Join(uniqueFacts, "\n- ")
	userPrompt := fmt.Sprintf(canonSystemPrompt, fandomSlug, "- "+factsStr, task.Payload, warnings)

	// 5. Отправляем в GigaChat
	llmResponse, usage, err := u.llm.SendChat(gigachat.ModelGigaChatMax, "Ты строгий критик. Отвечай только на русском языке.", userPrompt)
	if err != nil {
		log.Error().
			Err(err).
			Str("trace_id", task.TraceID).
			Str("task_id", task.TaskID).
			Msg("Ошибка запроса канон-чека к GigaChat")
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	cleanResponse := strings.ReplaceAll(llmResponse, "**", "")
	cleanResponse = strings.ReplaceAll(cleanResponse, "*", "")

	resultMap := map[string]string{
		"message": strings.TrimSpace(cleanResponse),
	}

	return u.publishResultWithTokens(ctx, task, "completed", resultMap, "", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
}

const summarySystemPrompt = `Ты — профессиональный книжный редактор. 
Напиши интригующее описание (аннотацию) для истории на основе текста первой главы. 
Опиши завязку, атмосферу и главного героя. НЕ раскрывай сюжетные повороты, интригу и концовку!
Максимум 3-4 предложения, не больше 180 символов. Отвечай только текстом аннотации.`

func (u *AIUsecase) processGenerateSummary(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	storyID, _ := uuid.Parse(task.Metadata["story_id"])

	_ = u.repo.CreateTask(ctx, taskID, "generate_summary", "story_id: "+storyID.String())

	summary, usage, err := u.llm.SendChat(gigachat.ModelGigaChat, summarySystemPrompt, "Текст первой главы:\n"+task.Payload)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}
	summary = strings.TrimSpace(summary)

	embedding, err := u.embeddings.GetEmbedding(ctx, summary)
	if err != nil {
		_ = u.repo.UpdateSummary(ctx, storyID, summary)
		return u.publishResultWithTokens(ctx, task, "failed", nil, "embedding error: "+err.Error(), usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	if err := u.repo.UpdateSummaryAndEmbedding(ctx, storyID, summary, embedding); err != nil {
		return u.publishResultWithTokens(ctx, task, "failed", nil, "failed to save summary: "+err.Error(), usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	return u.publishResultWithTokens(ctx, task, "completed", map[string]string{"summary": summary}, "", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
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

	llmResponse, usage, err := u.llm.SendChat(gigachat.ModelGigaChatMax, "Ты — строгий бета-ридер. Отвечай только на русском языке, без форматирования и эмодзи.", userPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	cleanResponse := strings.ReplaceAll(llmResponse, "**", "")
	cleanResponse = strings.ReplaceAll(cleanResponse, "*", "")

	return u.publishResultWithTokens(ctx, task, "completed", map[string]string{
		"message": strings.TrimSpace(cleanResponse),
	}, "", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
}

const fandomLoreSystemPrompt = `Ты — эксперт-архивариус вымышленных миров. Твоя задача: на основе текста описания сгенерировать структурированные факты для канона. Отвечай только на русском языке.`

func (u *AIUsecase) processGenerateFandomLore(ctx context.Context, task sharedrmq.MLTaskMessage) error {
	taskID, _ := uuid.Parse(task.TaskID)
	fandomSlug := task.Metadata["fandom_slug"]

	_ = u.repo.CreateTask(ctx, taskID, "generate_fandom_lore", "fandom: "+fandomSlug)

	llmResponse, usage, err := u.llm.SendChat(gigachat.ModelGigaChatPro, fandomLoreSystemPrompt, "Описание вселенной:\n"+task.Payload)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	cleanJSONStr := cleanJSON(llmResponse)

	var rawFacts []map[string]string
	if err := json.Unmarshal([]byte(cleanJSONStr), &rawFacts); err != nil {
		return u.publishResultWithTokens(ctx, task, "failed", nil, "invalid json from llm: "+err.Error(), usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	inserted := 0
	for _, item := range rawFacts {
		entity := item["entity"]
		fact := item["fact"]

		if entity == "" || fact == "" {
			continue
		}

		vector, err := u.embeddings.GetEmbedding(ctx, fact)
		if err != nil {
			continue
		}

		canonFact := models.CanonFact{
			ID:         uuid.New(),
			FandomSlug: fandomSlug,
			EntityName: entity,
			FactText:   fact,
			Embedding:  vector,
		}

		if err := u.repo.InsertCanonFact(ctx, canonFact); err == nil {
			inserted++
		}
	}

	resultMsg := map[string]any{
		"message":        "Фэндом успешно обработан",
		"facts_inserted": inserted,
	}

	return u.publishResultWithTokens(ctx, task, "completed", resultMsg, "", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
}

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

	llmResponse, usage, err := u.llm.SendChat(gigachat.ModelGigaChatPro, iterativeLoreSystemPrompt, userPrompt)
	if err != nil {
		return u.publishResult(ctx, task, "failed", nil, "gigachat error: "+err.Error())
	}

	cleanJSONStr := cleanJSON(llmResponse)
	if !json.Valid([]byte(cleanJSONStr)) {
		return u.publishResultWithTokens(ctx, task, "failed", nil, "gigachat returned invalid JSON", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	if err := u.repo.UpsertChapterLorebook(ctx, chapterID, storyID, contentHash, cleanJSONStr); err != nil {
		return u.publishResultWithTokens(ctx, task, "failed", nil, "failed to save chapter lore: "+err.Error(), usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}

	// Сбрасываем словарь исключений для фанфика, так как его лор обновился!
	u.vocabManager.InvalidateStoryVocabulary(storyID)

	return u.publishResultWithTokens(ctx, task, "completed", json.RawMessage(cleanJSONStr), "", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
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

func sha256Hex(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
