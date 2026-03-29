package languagetool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CheckResponse struct {
	Matches []Match `json:"matches"`
}

type Match struct {
	Message      string        `json:"message"`
	Offset       int           `json:"offset"`
	Length       int           `json:"length"`
	Replacements []Replacement `json:"replacements"`
}

type Replacement struct {
	Value string `json:"value"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Check(ctx context.Context, text string) (CheckResponse, error) {
	data := url.Values{}
	data.Set("text", text)
	data.Set("language", "ru-RU")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v2/check", strings.NewReader(data.Encode()))
	if err != nil {
		return CheckResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CheckResponse{}, fmt.Errorf("languagetool request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CheckResponse{}, fmt.Errorf("languagetool returned status %d", resp.StatusCode)
	}

	var res CheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return CheckResponse{}, fmt.Errorf("failed to decode languagetool response: %w", err)
	}

	return res, nil
}
