package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
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
