package embedding

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenAIEmbed(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"input":["one","two"],"model":"test-model"}` && string(body) != `{"model":"test-model","input":["one","two"]}` {
			t.Fatalf("unexpected body: %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(`{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}]}`)), Header: make(http.Header)}, nil
	})}
	p, err := NewOpenAI(OpenAIConfig{BaseURL: "https://provider.test/v1", APIKey: "test-key", Model: "test-model", Dim: 2, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := p.Embed(context.Background(), []string{"one", "two"})
	if err != nil || vectors[0][0] != 1 || vectors[1][0] != 3 {
		t.Fatalf("Embed() = %#v, %v", vectors, err)
	}
}
