package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

type SpellcheckItem struct {
	FragmentText string `json:"fragmentText"`
	StartOffset  int    `json:"startOffset"`
	EndOffset    int    `json:"endOffset"`
	Message      string `json:"message"`
	Suggestion   string `json:"suggestion"`
}

type SpellcheckResult struct {
	Summary string           `json:"summary"`
	Items   []SpellcheckItem `json:"items"`
}

type ImageGenItem struct {
	BinaryOrURL string `json:"binaryOrUrl"`
	Prompt      string `json:"prompt"`
}

type ImageGenResult struct {
	Images []ImageGenItem `json:"images"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Spellcheck(ctx context.Context, content string) (SpellcheckResult, error) {
	if c.baseURL == "" {
		return SpellcheckResult{}, fmt.Errorf("ml: base URL is not configured")
	}
	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return SpellcheckResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/spellcheck", bytes.NewReader(body))
	if err != nil {
		return SpellcheckResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SpellcheckResult{}, fmt.Errorf("ml spellcheck request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return SpellcheckResult{}, fmt.Errorf("ml spellcheck read body: %w", err)
	}
	if resp.StatusCode >= 300 {
		return SpellcheckResult{}, fmt.Errorf("ml spellcheck: %s", resp.Status)
	}
	var out SpellcheckResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return SpellcheckResult{}, fmt.Errorf("ml spellcheck decode: %w", err)
	}
	return out, nil
}

func (c *Client) GenerateImage(ctx context.Context, content, prompt string) (ImageGenResult, error) {
	if c.baseURL == "" {
		return ImageGenResult{}, fmt.Errorf("ml: base URL is not configured")
	}
	body, err := json.Marshal(map[string]string{
		"content": content,
		"prompt":  prompt,
	})
	if err != nil {
		return ImageGenResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/image-generation", bytes.NewReader(body))
	if err != nil {
		return ImageGenResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ImageGenResult{}, fmt.Errorf("ml image-generation request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ImageGenResult{}, fmt.Errorf("ml image-generation read body: %w", err)
	}
	if resp.StatusCode >= 300 {
		return ImageGenResult{}, fmt.Errorf("ml image-generation: %s", resp.Status)
	}
	var out ImageGenResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return ImageGenResult{}, fmt.Errorf("ml image-generation decode: %w", err)
	}
	return out, nil
}

func (c *Client) GetWiki(ctx context.Context, chapterID uuid.UUID) (json.RawMessage, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("ml: base URL is not configured")
	}
	if chapterID == uuid.Nil {
		return []byte("{}"), nil
	}

	url := fmt.Sprintf("%s/internal/wiki?chapter_id=%s", c.baseURL, chapterID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ml get wiki request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ml get wiki read body: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ml get wiki: %s", resp.Status)
	}

	return json.RawMessage(raw), nil
}

func (c *Client) GetSimilarStories(ctx context.Context, storyID uuid.UUID) ([]uuid.UUID, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("ml: base URL is not configured")
	}

	url := fmt.Sprintf("%s/internal/similar?story_id=%s", c.baseURL, storyID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ml get similar request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var ids []uuid.UUID
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, err
	}

	return ids, nil
}

func (c *Client) SearchSemantic(ctx context.Context, query string) ([]uuid.UUID, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("ml: base URL is not configured")
	}

	url := fmt.Sprintf("%s/internal/search_semantic?q=%s", c.baseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ml search semantic request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ml search semantic returned: %s", resp.Status)
	}

	var ids []uuid.UUID
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, err
	}

	return ids, nil
}
