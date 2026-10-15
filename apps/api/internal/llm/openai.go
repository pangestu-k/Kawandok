package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const openaiName = "openai"

type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type OpenAI struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAI(cfg OpenAIConfig) (*OpenAI, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil, fmt.Errorf("%w: openai provider requires base url, api key and model", ErrInvalidRequest)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OpenAI{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (o *OpenAI) Name() string { return openaiName }

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openaiResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message openaiMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type openaiStreamChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (o *OpenAI) toWireMessages(req CompleteRequest) ([]openaiMessage, error) {
	if len(req.Tools) > 0 {
		return nil, fmt.Errorf("%w: openai adapter does not support tools yet", ErrInvalidRequest)
	}
	msgs := make([]openaiMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, openaiMessage{Role: string(m.Role), Content: m.Content})
	}
	return msgs, nil
}

func (o *OpenAI) completeOnce(ctx context.Context, req CompleteRequest, stream bool) (io.ReadCloser, error) {
	msgs, err := o.toWireMessages(req)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(openaiRequest{Model: o.model, Messages: msgs, Stream: stream})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	httpResp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		snippet, _ := io.ReadAll(io.LimitReader(httpResp.Body, 512))
		return nil, fmt.Errorf("openai returned status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return httpResp.Body, nil
}

func (o *OpenAI) Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return CompleteResponse{}, err
	}
	if err := req.Validate(); err != nil {
		return CompleteResponse{}, err
	}
	body, err := o.completeOnce(ctx, req, false)
	if err != nil {
		return CompleteResponse{}, err
	}
	defer body.Close()

	var parsed openaiResponse
	if err := json.NewDecoder(body).Decode(&parsed); err != nil {
		return CompleteResponse{}, fmt.Errorf("openai decode: %w", err)
	}
	if parsed.Error != nil {
		return CompleteResponse{}, fmt.Errorf("openai error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return CompleteResponse{}, errors.New("openai returned no choices")
	}
	choice := parsed.Choices[0]
	return CompleteResponse{
		Model:        parsed.Model,
		Message:      Message{Role: RoleAssistant, Content: choice.Message.Content},
		FinishReason: "stop",
	}, nil
}

func (o *OpenAI) Stream(ctx context.Context, req CompleteRequest, fn StreamFunc) (CompleteResponse, error) {
	if fn == nil {
		return CompleteResponse{}, fmt.Errorf("%w: stream function is nil", ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return CompleteResponse{}, err
	}
	if err := req.Validate(); err != nil {
		return CompleteResponse{}, err
	}
	body, err := o.completeOnce(ctx, req, true)
	if err != nil {
		return CompleteResponse{}, err
	}
	defer body.Close()

	var content strings.Builder
	servedModel := o.model
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return CompleteResponse{}, err
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				break
			}
			continue
		}
		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return CompleteResponse{}, fmt.Errorf("openai stream decode: %w", err)
		}
		if chunk.Model != "" {
			servedModel = chunk.Model
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				if err := fn(choice.Delta.Content); err != nil {
					return CompleteResponse{}, err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return CompleteResponse{}, fmt.Errorf("openai stream read: %w", err)
	}
	return CompleteResponse{
		Model:        servedModel,
		Message:      Message{Role: RoleAssistant, Content: content.String()},
		FinishReason: "stop",
	}, nil
}
