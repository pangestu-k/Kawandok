package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRobinRotatesAcrossEndpoints(t *testing.T) {
	var hitsA, hitsB atomic.Int64
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA.Add(1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"dari-A"}}]}`))
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB.Add(1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"dari-B"}}]}`))
	}))
	defer serverB.Close()

	robin, err := NewRobin(RobinConfig{Entries: []RobinEntry{
		{BaseURL: serverA.URL, APIKey: "k", Model: "m-a"},
		{BaseURL: serverB.URL, APIKey: "k", Model: "m-b"},
	}})
	if err != nil {
		t.Fatalf("NewRobin() error: %v", err)
	}

	req := CompleteRequest{Messages: []Message{{Role: RoleUser, Content: "halo"}}}
	first, err := robin.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	second, err := robin.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}

	if hitsA.Load() != 1 || hitsB.Load() != 1 {
		t.Errorf("hits a=%d b=%d, want one request per endpoint", hitsA.Load(), hitsB.Load())
	}
	if first.Message.Content == second.Message.Content {
		t.Error("round robin must alternate endpoints")
	}
}

func TestRobinFallsBackWhenEndpointFails(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"cadangan"}}]}`))
	}))
	defer healthy.Close()

	robin, err := NewRobin(RobinConfig{Entries: []RobinEntry{
		{BaseURL: broken.URL, APIKey: "k", Model: "m-broken"},
		{BaseURL: healthy.URL, APIKey: "k", Model: "m-healthy"},
	}})
	if err != nil {
		t.Fatalf("NewRobin() error: %v", err)
	}

	resp, err := robin.Complete(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: "halo"}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Message.Content != "cadangan" {
		t.Errorf("content = %q, want fallback answer", resp.Message.Content)
	}
}

func TestRobinReportsErrorWhenAllEndpointsFail(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer broken.Close()
	broken2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer broken2.Close()

	robin, err := NewRobin(RobinConfig{Entries: []RobinEntry{
		{BaseURL: broken.URL, APIKey: "k", Model: "m1"},
		{BaseURL: broken2.URL, APIKey: "k", Model: "m2"},
	}})
	if err != nil {
		t.Fatalf("NewRobin() error: %v", err)
	}
	if _, err := robin.Complete(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: "halo"}},
	}); err == nil {
		t.Fatal("all endpoints failing must return an error")
	}
}

func TestRobinStreamFallsBackWhenFirstNeverEmits(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()

	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer live.Close()

	robin, err := NewRobin(RobinConfig{Entries: []RobinEntry{
		{BaseURL: broken.URL, APIKey: "k", Model: "m1"},
		{BaseURL: live.URL, APIKey: "k", Model: "m2"},
	}})
	if err != nil {
		t.Fatalf("NewRobin() error: %v", err)
	}

	var got strings.Builder
	if _, err := robin.Stream(context.Background(), CompleteRequest{
		Messages: []Message{{Role: RoleUser, Content: "halo"}},
	}, func(delta string) error {
		got.WriteString(delta)
		return nil
	}); err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if got.String() != "ok" {
		t.Errorf("streamed = %q, want ok", got.String())
	}
}

func TestParseRobinEndpoints(t *testing.T) {
	entries, err := ParseRobinEndpoints(
		"https://a/v1|key-a|model-a ; https://b/v1|key-b|model-b")
	if err != nil {
		t.Fatalf("ParseRobinEndpoints() error: %v", err)
	}
	if len(entries) != 2 || entries[0].Model != "model-a" || entries[1].APIKey != "key-b" {
		t.Errorf("entries = %+v", entries)
	}

	if _, err := ParseRobinEndpoints("https://bad|only-two"); err == nil {
		t.Error("malformed entry must be rejected")
	}
	if _, err := ParseRobinEndpoints("||"); err == nil {
		t.Error("empty fields must be rejected")
	}
}

func TestBuildRobinGroupAndInterleave(t *testing.T) {
	groq := BuildRobinGroup("https://groq/v1", "key-groq", "m1, m2 ,m3")
	if len(groq) != 3 || groq[1].Model != "m2" || groq[0].BaseURL != "https://groq/v1" {
		t.Fatalf("BuildRobinGroup() = %+v", groq)
	}
	gemini := BuildRobinGroup("https://gemini/v1", "key-gemini", "g1,g2")
	chain := InterleaveRobinEntries(groq, gemini)
	want := []string{"m1", "g1", "m2", "g2", "m3"}
	if len(chain) != len(want) {
		t.Fatalf("chain length = %d, want %d", len(chain), len(want))
	}
	for i, model := range want {
		if chain[i].Model != model {
			t.Errorf("chain[%d].Model = %q, want %q", i, chain[i].Model, model)
		}
	}
	if got := ParseRobinModels(" a , ,b "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("ParseRobinModels() = %v", got)
	}
}
