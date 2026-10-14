package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Dim     int
	Client  *http.Client
}

type OpenAI struct {
	baseURL string
	apiKey  string
	model   string
	dim     int
	client  *http.Client
}

func NewOpenAI(cfg OpenAIConfig) (*OpenAI, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" || cfg.Dim < 1 {
		return nil, fmt.Errorf("embedding: OpenAI base URL, API key, model, and dimension are required")
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &OpenAI{baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: cfg.APIKey, model: cfg.Model, dim: cfg.Dim, client: client}, nil
}

func (o *OpenAI) Name() string { return o.model }
func (o *OpenAI) Dim() int     { return o.dim }

func (o *OpenAI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("embedding: no texts provided")
	}
	body, err := json.Marshal(map[string]any{"model": o.model, "input": texts})
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("embedding: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding: provider returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("embedding: decode response: %w", err)
	}
	if len(payload.Data) != len(texts) {
		return nil, fmt.Errorf("embedding: provider returned %d vectors for %d texts", len(payload.Data), len(texts))
	}
	vectors := make([][]float32, len(texts))
	for _, item := range payload.Data {
		if item.Index < 0 || item.Index >= len(vectors) || vectors[item.Index] != nil || len(item.Embedding) != o.dim {
			return nil, fmt.Errorf("embedding: invalid provider response")
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}
