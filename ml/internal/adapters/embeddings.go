package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type EmbeddingsClient struct {
	baseURL string
	client  *http.Client
}

func NewEmbeddingsClient(baseURL string) *EmbeddingsClient {
	return &EmbeddingsClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *EmbeddingsClient) GetEmbedding(ctx context.Context, text string) ([]float32, error) {
	reqBody, _ := json.Marshal(map[string]string{"text": text})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/embed", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings service returned status: %d", resp.StatusCode)
	}

	var res struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return res.Embedding, nil
}
