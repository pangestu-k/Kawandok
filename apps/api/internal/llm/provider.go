package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role    Role
	Content string
}

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
}

type CompleteRequest struct {
	Messages    []Message
	Tools       []Tool
	MaxTokens   int
	Temperature float64
}

var ErrInvalidRequest = errors.New("llm: invalid request")

func (r CompleteRequest) Validate() error {
	if len(r.Messages) == 0 {
		return fmt.Errorf("%w: messages is empty", ErrInvalidRequest)
	}
	for i, m := range r.Messages {
		if m.Role == "" {
			return fmt.Errorf("%w: message %d has empty role", ErrInvalidRequest, i)
		}
		if m.Content == "" {
			return fmt.Errorf("%w: message %d has empty content", ErrInvalidRequest, i)
		}
	}
	return nil
}

type CompleteResponse struct {
	// Model is the model that actually served the request. Providers that
	// round-robin report the serving entry, not the whole chain.
	Model        string
	Message      Message
	ToolCalls    []ToolCall
	Usage        Usage
	FinishReason string
}

type StreamFunc func(delta string) error

type Provider interface {
	Name() string
	Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error)
	Stream(ctx context.Context, req CompleteRequest, fn StreamFunc) (CompleteResponse, error)
}
