package llm

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func sampleRequest() CompleteRequest {
	return CompleteRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are a helpful office assistant."},
			{Role: RoleUser, Content: "Where is the leave policy?"},
		},
	}
}

func TestMockCompleteIsDeterministic(t *testing.T) {
	mock := NewMock()
	first, err := mock.Complete(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	second, err := mock.Complete(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("responses differ:\nfirst:  %+v\nsecond: %+v", first, second)
	}
	if first.Message.Role != RoleAssistant {
		t.Errorf("role = %q, want assistant", first.Message.Role)
	}
	if first.Usage.InputTokens <= 0 || first.Usage.OutputTokens <= 0 {
		t.Errorf("usage must be positive, got %+v", first.Usage)
	}
	if !strings.Contains(first.Message.Content, "2 message(s)") {
		t.Errorf("content %q should mention message count", first.Message.Content)
	}
}

func TestMockCompleteListsTools(t *testing.T) {
	mock := NewMock()
	req := sampleRequest()
	req.Tools = []Tool{
		{Name: "search_documents"},
		{Name: "get_financial_summary"},
	}
	resp, err := mock.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if !strings.Contains(resp.Message.Content, "tools:search_documents,get_financial_summary") {
		t.Errorf("content %q should list tools", resp.Message.Content)
	}
}

func TestMockCompleteRejectsInvalidRequests(t *testing.T) {
	mock := NewMock()

	if _, err := mock.Complete(context.Background(), CompleteRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("empty messages: got %v, want ErrInvalidRequest", err)
	}

	req := CompleteRequest{Messages: []Message{{Role: RoleSystem, Content: "hi"}}}
	if _, err := mock.Complete(context.Background(), req); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("assistant-last: got %v, want ErrInvalidRequest", err)
	}

	req = CompleteRequest{Messages: []Message{{Role: RoleUser, Content: ""}}}
	if _, err := mock.Complete(context.Background(), req); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("empty content: got %v, want ErrInvalidRequest", err)
	}
}

func TestMockStreamConcatenatesToCompleteContent(t *testing.T) {
	mock := NewMock()
	want, err := mock.Complete(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}

	var sb strings.Builder
	got, err := mock.Stream(context.Background(), sampleRequest(), func(delta string) error {
		sb.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if sb.String() != want.Message.Content {
		t.Errorf("streamed %q, want %q", sb.String(), want.Message.Content)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stream response %+v differs from complete response %+v", got, want)
	}
}

func TestMockStreamPropagatesHandlerError(t *testing.T) {
	mock := NewMock()
	sentinel := errors.New("handler failed")
	_, err := mock.Stream(context.Background(), sampleRequest(), func(delta string) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("Stream() error = %v, want handler error", err)
	}
}

func TestMockStreamNilFunction(t *testing.T) {
	mock := NewMock()
	if _, err := mock.Stream(context.Background(), sampleRequest(), nil); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Stream() with nil fn: got %v, want ErrInvalidRequest", err)
	}
}

func TestMockRespectsCancelledContext(t *testing.T) {
	mock := NewMock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := mock.Complete(ctx, sampleRequest()); !errors.Is(err, context.Canceled) {
		t.Errorf("Complete() error = %v, want context.Canceled", err)
	}
	if _, err := mock.Stream(ctx, sampleRequest(), func(string) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("Stream() error = %v, want context.Canceled", err)
	}
}

func TestMockName(t *testing.T) {
	if got := NewMock().Name(); got != "mock" {
		t.Errorf("Name() = %q, want mock", got)
	}
}
