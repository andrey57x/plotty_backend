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
)

// Регулярное выражение для поиска ID файла в ответе Kandinsky
var imgTagRegex = regexp.MustCompile(`<img src="([^"]+)"`)

type Client struct {
	authKey     string
	token       string
	expiresAt   time.Time
	httpClient  *http.Client // Для текстовых запросов (120 секунд)
	imageClient *http.Client // Для быстрой генерации изображений (20 секунд)
	mu          sync.Mutex   // Для обновления токена
	apiMu       sync.Mutex   // Для сериализации всех API запросов (лимит 1 поток у физлиц!)
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"`
}

func NewClient(authKey string) *Client {
	customTransport := http.DefaultTransport.(*http.Transport).Clone()
	customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	return &Client{
		authKey: authKey,
		httpClient: &http.Client{
			Transport: customTransport,
			Timeout:   standartTimeout * time.Second, // Оставляем стандартный таймаут для текста
		},
		imageClient: &http.Client{
			Transport: customTransport,
			Timeout:   imageTimeout * time.Second, // Ограничиваем таймаут для картинок до 20 секунд
		},
	}
}

func (c *Client) ensureToken() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Add(1*time.Minute).Before(c.expiresAt) {
		return nil
	}

	payload := strings.NewReader("scope=GIGACHAT_API_PERS")
	req, err := http.NewRequest("POST", authURL, payload)
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("RqUID", uuid.New().String())
	req.Header.Add("Authorization", "Basic "+c.authKey)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("auth failed: %s", string(body))
	}

	var tResp tokenResponse
	if err := json.NewDecoder(res.Body).Decode(&tResp); err != nil {
		return err
	}

	c.token = tResp.AccessToken
	c.expiresAt = time.Unix(0, tResp.ExpiresAt*int64(time.Millisecond))
	return nil
}

// SendChat отправляет текстовый запрос с указанием конкретной модели
func (c *Client) SendChat(modelName, systemPrompt, userText string) (string, error) {
	c.apiMu.Lock()
	defer c.apiMu.Unlock()

	if err := c.ensureToken(); err != nil {
		return "", err
	}

	reqBody := ChatRequest{
		Model: modelName,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userText},
		},
		Temperature: completionTemperature,
	}

	return c.doChatRequest(reqBody)
}

// GenerateImage отправляет запрос на генерацию изображения и возвращает file_id
func (c *Client) GenerateImage(prompt string) (string, error) {
	c.apiMu.Lock()
	defer c.apiMu.Unlock()

	if err := c.ensureToken(); err != nil {
		return "", err
	}

	reqBody := ChatRequest{
		Model: "GigaChat",
		Messages: []Message{
			{Role: "user", Content: "Нарисуй: " + prompt},
		},
		Temperature:  0.7,
		FunctionCall: "auto", // ТА САМАЯ НАСТРОЙКА ИЗ ДОКУМЕНТАЦИИ
	}

	content, err := c.doImageChatRequest(reqBody)
	if err != nil {
		return "", err
	}

	// Извлекаем file_id из тега <img src="...">
	match := imgTagRegex.FindStringSubmatch(content)
	if len(match) < 2 {
		return "", fmt.Errorf("file_id not found in response: %s", content)
	}

	return match[1], nil
}

// DownloadFile скачивает байты файла по его ID
func (c *Client) DownloadFile(fileID string) ([]byte, error) {
	c.apiMu.Lock()
	defer c.apiMu.Unlock()

	if err := c.ensureToken(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf(fileURL, fileID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+c.token)

	res, err := c.imageClient.Do(req) // Используем imageClient
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("download failed: %s", string(body))
	}

	return io.ReadAll(res.Body)
}

func (c *Client) doChatRequest(reqBody ChatRequest) (string, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", chatURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+c.token)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var chatResp ChatResponse
	if err := json.NewDecoder(res.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty choices from gigachat")
	}

	return chatResp.Choices[0].Message.Content, nil
}

func (c *Client) doImageChatRequest(reqBody ChatRequest) (string, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", chatURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+c.token)

	res, err := c.imageClient.Do(req) // Используем imageClient для картинок
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var chatResp ChatResponse
	if err := json.NewDecoder(res.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty choices from gigachat")
	}

	return chatResp.Choices[0].Message.Content, nil
}
