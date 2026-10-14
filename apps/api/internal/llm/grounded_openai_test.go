package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func newOpenAITestServer(t *testing.T, stream bool, handler http.HandlerFunc) (*OpenAI, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	provider, err := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatalf("NewOpenAI() error: %v", err)
	}
	_ = stream
	return provider, srv
}

func TestOpenAIComplete(t *testing.T) {
	provider, _ := newOpenAITestServer(t, false, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer token")
		}
		var req openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		if req.Stream {
			t.Errorf("stream must be false for Complete")
		}
		if req.Model != "test-model" {
			t.Errorf("model = %q", req.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"jawaban uji"}}]}`))
	})

	resp, err := provider.Complete(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: "halo"}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Message.Content != "jawaban uji" {
		t.Errorf("content = %q", resp.Message.Content)
	}
}

func TestOpenAIStreamReassemblesDeltas(t *testing.T) {
	provider, _ := newOpenAITestServer(t, true, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, payload := range []string{
			`{"choices":[{"delta":{"content":"satu "}}]}`,
			`{"choices":[{"delta":{"content":"dua"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`[DONE]`,
		} {
			_, _ = w.Write([]byte("data: " + payload + "\n\n"))
			flusher.Flush()
		}
	})

	var sb strings.Builder
	resp, err := provider.Stream(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: "halo"}},
	}, func(delta string) error {
		sb.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if sb.String() != "satu dua" {
		t.Errorf("streamed = %q, want %q", sb.String(), "satu dua")
	}
	if resp.Message.Content != "satu dua" {
		t.Errorf("final content = %q", resp.Message.Content)
	}
}

func TestOpenAIHTTPErrorSurfaces(t *testing.T) {
	provider, _ := newOpenAITestServer(t, false, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	})
	_, err := provider.Complete(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: "halo"}},
	})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("error = %v, want status 429 mention", err)
	}
}

func TestOpenAIRequiresConfiguration(t *testing.T) {
	if _, err := NewOpenAI(OpenAIConfig{BaseURL: "http://x"}); err == nil {
		t.Error("missing key/model must be rejected")
	}
}

func TestOpenAIRejectsTools(t *testing.T) {
	provider, _ := newOpenAITestServer(t, false, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request must not be sent when tools are rejected upfront")
	})
	_, err := provider.Complete(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: "halo"}},
		Tools:    []Tool{{Name: "search_documents"}},
	})
	if err == nil {
		t.Error("tools must be rejected by the M3 adapter")
	}
}

func TestGroundedMockUncertaintyWithoutEvidence(t *testing.T) {
	mock := NewGroundedMock()
	resp, err := mock.Complete(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: "Pertanyaan tanpa evidence."}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Message.Content != uncertaintyReply {
		t.Errorf("content = %q, want uncertainty reply", resp.Message.Content)
	}
}

func TestGroundedMockCitesEvidence(t *testing.T) {
	mock := NewGroundedMock()
	prompt := "Pertanyaan: berapa total revenue?\n\n" +
		"[EVIDENCE 1 id=chunk-a]\nTotal revenue January is 12 million IDR.\n" +
		"[EVIDENCE 2 id=chunk-b]\nTransport allowance is 5 million IDR."
	resp, err := mock.Complete(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: prompt}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if !strings.Contains(resp.Message.Content, "12 million IDR") {
		t.Errorf("grounded content missing evidence: %q", resp.Message.Content)
	}
	if !strings.Contains(resp.Message.Content, "[1]") || !strings.Contains(resp.Message.Content, "[2]") {
		t.Errorf("content missing citations: %q", resp.Message.Content)
	}
	if strings.Contains(resp.Message.Content, "Pertanyaan") {
		t.Error("question text must not leak into evidence body")
	}
	again := mustComplete(t, mock, CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: prompt}},
	})
	if !reflect.DeepEqual(resp, again) {
		t.Error("grounded mock must be deterministic")
	}
}

func TestGroundedMockStreamMatchesComplete(t *testing.T) {
	mock := NewGroundedMock()
	req := CompleteRequest{Messages: []Message{{Role: RoleUser, Content: "Pertanyaan?\n\n[EVIDENCE 1 id=x]\nisi evidence."}}}
	want, err := mock.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	var sb strings.Builder
	if _, err := mock.Stream(context.Background(), req, func(d string) error {
		sb.WriteString(d)
		return nil
	}); err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if sb.String() != want.Message.Content {
		t.Errorf("stream %q != complete %q", sb.String(), want.Message.Content)
	}
}

func mustComplete(t *testing.T, p Provider, req CompleteRequest) CompleteResponse {
	t.Helper()
	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	return resp
}
