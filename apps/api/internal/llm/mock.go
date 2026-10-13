package llm

import (
	"context"
	"fmt"
	"strings"
)

const (
	mockName          = "mock"
	mockMaxReplyChars = 400
	mockStreamStride  = 7
)

type Mock struct {
	maxReplyChars int
}

func NewMock() *Mock {
	return &Mock{maxReplyChars: mockMaxReplyChars}
}

func (m *Mock) Name() string {
	return mockName
}

func (m *Mock) Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return CompleteResponse{}, err
	}
	if err := req.Validate(); err != nil {
		return CompleteResponse{}, err
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role != RoleUser && last.Role != RoleTool {
		return CompleteResponse{}, fmt.Errorf("%w: cannot answer message with role %q", ErrInvalidRequest, last.Role)
	}
	reply := m.reply(req)
	return CompleteResponse{
		Message:      Message{Role: RoleAssistant, Content: reply},
		Usage:        Usage{InputTokens: estimateInputTokens(req), OutputTokens: len(strings.Fields(reply))},
		FinishReason: "stop",
	}, nil
}

func (m *Mock) Stream(ctx context.Context, req CompleteRequest, fn StreamFunc) (CompleteResponse, error) {
	if fn == nil {
		return CompleteResponse{}, fmt.Errorf("%w: stream function is nil", ErrInvalidRequest)
	}
	resp, err := m.Complete(ctx, req)
	if err != nil {
		return CompleteResponse{}, err
	}
	content := resp.Message.Content
	for i := 0; i < len(content); i += mockStreamStride {
		if err := ctx.Err(); err != nil {
			return CompleteResponse{}, err
		}
		end := i + mockStreamStride
		if end > len(content) {
			end = len(content)
		}
		if err := fn(content[i:end]); err != nil {
			return CompleteResponse{}, err
		}
	}
	return resp, nil
}

func (m *Mock) reply(req CompleteRequest) string {
	last := req.Messages[len(req.Messages)-1]
	toolNote := "no tools"
	if len(req.Tools) > 0 {
		names := make([]string, 0, len(req.Tools))
		for _, t := range req.Tools {
			names = append(names, t.Name)
		}
		toolNote = "tools:" + strings.Join(names, ",")
	}
	input := last.Content
	if len(input) > m.maxReplyChars {
		input = input[:m.maxReplyChars]
	}
	return fmt.Sprintf("[mock] answered %d message(s) using %s; last input: %s", len(req.Messages), toolNote, input)
}

func estimateInputTokens(req CompleteRequest) int {
	words := 0
	for _, msg := range req.Messages {
		words += len(strings.Fields(msg.Content)) + 1
	}
	return words
}
