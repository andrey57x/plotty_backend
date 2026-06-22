package gigachat

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log" // используем логгер для вывода ротации на проде
)

// Регулярное выражение для поиска ID файла в ответе Kandinsky
var imgTagRegex = regexp.MustCompile(`<img src="([^"]+)"`)

type Client struct {
	authKeys    []string             // Слайс всех доступных ключей авторизации
	currentIdx  int                  // Индекс текущего активного ключа
	tokens      map[string]string    // Кэш токенов доступа для каждого ключа
	expiresAt   map[string]time.Time // Срок действия токена для каждого ключа
	httpClient  *http.Client         // Для текстовых запросов (120 секунд)
	imageClient *http.Client         // Для быстрой генерации изображений (20 секунд)
	mu          sync.Mutex           // Мьютекс для обновления токенов
	apiMu       sync.Mutex           // Мьютекс для синхронизации запросов к API Сбера
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"`
}

func NewClient(authKeyStr string) *Client {
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	// Разбираем строку ключей через запятую (поддерживает как один ключ, так и список)
	var keys []string
	for _, k := range strings.Split(authKeyStr, ",") {
		trimmed := strings.TrimSpace(k)
		if trimmed != "" {
			keys = append(keys, trimmed)
		}
	}

	if len(keys) == 0 {
		log.Fatal().Msg("GigaChat Client: инициализация невозможна, список ключей пуст")
	}

	return &Client{
		authKeys:   keys,
		currentIdx: 0,
		tokens:     make(map[string]string),
		expiresAt:  make(map[string]time.Time),
		httpClient: &http.Client{
			Transport: customTransport,
		},
		imageClient: &http.Client{
			Transport: customTransport,
			Timeout:   20 * time.Second,
		},
	}
}

// ensureToken гарантирует валидный токен для текущего активного ключа
func (c *Client) ensureToken() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	activeKey := c.authKeys[c.currentIdx]
	now := time.Now().UTC()

	// Если токен для этого ключа еще свежий — используем его
	if _, ok := c.tokens[activeKey]; ok && now.Before(c.expiresAt[activeKey]) {
		return nil
	}

	req, err := http.NewRequest("POST", authURL, strings.NewReader("scope=GIGACHAT_API_PERS"))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("RqUID", uuid.New().String())
	req.Header.Add("Authorization", "Basic "+activeKey)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("auth failed for key index %d (status %d): %s", c.currentIdx, res.StatusCode, string(body))
	}

	var tResp tokenResponse
	if err := json.NewDecoder(res.Body).Decode(&tResp); err != nil {
		return err
	}

	c.tokens[activeKey] = tResp.AccessToken
	c.expiresAt[activeKey] = time.Unix(0, tResp.ExpiresAt*int64(time.Millisecond))
	return nil
}

// rotateKey переключает клиент на следующий ключ по кругу
func (c *Client) rotateKey() {
	c.mu.Lock()
	oldIdx := c.currentIdx
	c.currentIdx = (c.currentIdx + 1) % len(c.authKeys)
	c.mu.Unlock()

	log.Warn().
		Int("old_index", oldIdx).
		Int("new_index", c.currentIdx).
		Msg("GigaChat Client: лимит исчерпан или превышен RPM. Произведена автоматическая ротация ключа.")
}

// getFallbackModel возвращает резервную модель, если текущая недоступна
func getFallbackModel(currentModel string) string {
	switch currentModel {
	case ModelGigaChatMax:
		return ModelGigaChatPro // если упал Max, откатываемся на Pro
	case ModelGigaChatPro:
		return ModelGigaChat // если упал Pro, откатываемся на базовый Lite
	default:
		return "" // базовую модель заменять нечем
	}
}

// SendChat — основная точка входа. Выполняет автоматическую ротацию ключей и фолбек моделей при ошибках 402/429
func (c *Client) SendChat(modelName, systemPrompt, userText string) (string, Usage, error) {
	c.apiMu.Lock()
	defer c.apiMu.Unlock()

	originalModel := modelName
	currentModel := modelName

	for {
		// Пробуем сделать запрос через все наши ключи по очереди
		for attempt := 0; attempt < len(c.authKeys); attempt++ {
			if err := c.ensureToken(); err != nil {
				// Если ключ не смог авторизоваться — ротируем его и пробуем следующий
				log.Error().Err(err).Int("key_index", c.currentIdx).Msg("GigaChat: ошибка авторизации ключа. Пробуем следующий.")
				c.rotateKey()
				continue
			}

			// Выполняем непосредственно сетевой запрос к Сберу
			activeKey := c.authKeys[c.currentIdx]
			activeToken := c.tokens[activeKey]

			content, usage, err := c.executeRequest(currentModel, systemPrompt, userText, activeToken)
			if err != nil {
				// Проверяем, является ли ошибка лимитом токенов (402) или лимитом RPM (429)
				if strings.Contains(err.Error(), "402") || strings.Contains(err.Error(), "429") {
					log.Warn().Err(err).Str("model", currentModel).Int("key_index", c.currentIdx).Msg("GigaChat: превышен лимит или закончились токены")
					c.rotateKey()
					continue
				}
				// Любая другая фатальная ошибка (например, 400 Bad Request) — возвращаем как есть
				return "", Usage{}, err
			}

			// Если запрос завершился успехом — возвращаем результат
			if currentModel != originalModel {
				log.Info().Str("original", originalModel).Str("fallback", currentModel).Msg("GigaChat: запрос успешно выполнен через резервную модель")
			}
			return content, usage, nil
		}

		// Если мы обошли ВСЕ ключи, и везде получили ошибки 402/429:
		// Пытаемся получить резервную модель
		fallback := getFallbackModel(currentModel)
		if fallback == "" {
			// Резервных моделей больше нет, возвращаем ошибку
			return "", Usage{}, fmt.Errorf("gigachat: все ключи авторизации и резервные модели исчерпали доступные лимиты (402/429)")
		}

		log.Warn().Str("failed_model", currentModel).Str("fallback_model", fallback).Msg("GigaChat: все ключи исчерпали лимиты для текущей модели. Переключаемся на резервную модель.")
		currentModel = fallback
		// Внешний цикл начнется заново с первой доступной ротации ключей, но уже на более простой резервной модели
	}
}

// executeRequest выполняет чистый POST запрос к API чата Сбера
func (c *Client) executeRequest(modelName, systemPrompt, userText, token string) (string, Usage, error) {
	reqBody := ChatRequest{
		Model: modelName,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userText},
		},
		Temperature: completionTemperature,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", Usage{}, err
	}

	req, err := http.NewRequest("POST", chatURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", Usage{}, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		// Возвращаем статус в ошибке, чтобы метод SendChat мог поймать 402/429 коды
		return "", Usage{}, fmt.Errorf("GigaChat API returned status %d: %s", res.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(res.Body).Decode(&chatResp); err != nil {
		return "", Usage{}, err
	}

	if len(chatResp.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("empty choices from gigachat")
	}

	return chatResp.Choices[0].Message.Content, chatResp.Usage, nil
}

// GenerateImage отправляет запрос на генерацию изображения и возвращает file_id
func (c *Client) GenerateImage(prompt string) (string, error) {
	c.apiMu.Lock()
	defer c.apiMu.Unlock()

	// Для генерации изображений используется только базовый GigaChat (Kandinsky).
	// Ротируем ключи, если первый доступный выдал ошибку.
	for attempt := 0; attempt < len(c.authKeys); attempt++ {
		if err := c.ensureToken(); err != nil {
			c.rotateKey()
			continue
		}

		activeKey := c.authKeys[c.currentIdx]
		activeToken := c.tokens[activeKey]

		reqBody := ChatRequest{
			Model: "GigaChat",
			Messages: []Message{
				{Role: "user", Content: "Нарисуй: " + prompt},
			},
			Temperature:  0.7,
			FunctionCall: "auto",
		}

		content, err := c.doImageChatRequestWithToken(reqBody, activeToken)
		if err != nil {
			if strings.Contains(err.Error(), "402") || strings.Contains(err.Error(), "429") {
				c.rotateKey()
				continue
			}
			return "", err
		}

		match := imgTagRegex.FindStringSubmatch(content)
		if len(match) < 2 {
			return "", fmt.Errorf("file_id not found in response: %s", content)
		}

		return match[1], nil
	}

	return "", fmt.Errorf("gigachat image gen: все ключи исчерпали доступные лимиты")
}

func (c *Client) doImageChatRequestWithToken(reqBody ChatRequest, token string) (string, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", chatURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)

	res, err := c.imageClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("status %d: %s", res.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(res.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty choices from gigachat")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// DownloadFile скачивает байты файла по его ID
func (c *Client) DownloadFile(fileID string) ([]byte, error) {
	c.apiMu.Lock()
	defer c.apiMu.Unlock()

	for attempt := 0; attempt < len(c.authKeys); attempt++ {
		if err := c.ensureToken(); err != nil {
			c.rotateKey()
			continue
		}

		activeKey := c.authKeys[c.currentIdx]
		activeToken := c.tokens[activeKey]

		url := fmt.Sprintf(fileURL, fileID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add("Authorization", "Bearer "+activeToken)

		res, err := c.imageClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			if res.StatusCode == http.StatusPaymentRequired || res.StatusCode == http.StatusTooManyRequests {
				c.rotateKey()
				continue
			}
			body, _ := io.ReadAll(res.Body)
			return nil, fmt.Errorf("download failed with status %d: %s", res.StatusCode, string(body))
		}

		return io.ReadAll(res.Body)
	}

	return nil, fmt.Errorf("gigachat download: все ключи исчерпали доступные лимиты")
}
