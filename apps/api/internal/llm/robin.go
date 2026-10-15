package llm

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// RobinEntry is one OpenAI-compatible endpoint in the round-robin chain.
type RobinEntry struct {
	BaseURL string
	APIKey  string
	Model   string
}

type RobinConfig struct {
	Entries []RobinEntry
	Timeout time.Duration
	// Logf, when set, receives every model attempt so operators can audit
	// which model served a request.
	Logf func(entry RobinEntry, err error)
}

// Robin rotates across OpenAI-compatible endpoints (round-robin per request)
// and falls through to the next entry when one fails.
type Robin struct {
	clients []*OpenAI
	counter atomic.Uint64
	timeout time.Duration
	logf    func(entry RobinEntry, err error)
}

func NewRobin(cfg RobinConfig) (*Robin, error) {
	if len(cfg.Entries) == 0 {
		return nil, fmt.Errorf("%w: robin requires at least one endpoint", ErrInvalidRequest)
	}
	clients := make([]*OpenAI, 0, len(cfg.Entries))
	for i, entry := range cfg.Entries {
		client, err := NewOpenAI(OpenAIConfig{
			BaseURL: entry.BaseURL,
			APIKey:  entry.APIKey,
			Model:   entry.Model,
			Timeout: cfg.Timeout,
		})
		if err != nil {
			return nil, fmt.Errorf("robin entry %d: %w", i+1, err)
		}
		clients = append(clients, client)
	}
	return &Robin{clients: clients, timeout: cfg.Timeout, logf: cfg.Logf}, nil
}

func (r *Robin) Name() string { return "robin" }

func (r *Robin) ordered() []*OpenAI {
	start := int((r.counter.Add(1) - 1) % uint64(len(r.clients)))
	ordered := make([]*OpenAI, 0, len(r.clients))
	for i := 0; i < len(r.clients); i++ {
		ordered = append(ordered, r.clients[(start+i)%len(r.clients)])
	}
	return ordered
}

func (r *Robin) Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return CompleteResponse{}, err
	}
	if err := req.Validate(); err != nil {
		return CompleteResponse{}, err
	}

	var lastErr error
	for _, client := range r.ordered() {
		resp, err := client.Complete(ctx, req)
		if r.logf != nil {
			r.logf(RobinEntry{BaseURL: client.baseURL, Model: client.model}, err)
		}
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return CompleteResponse{}, ctx.Err()
		}
		lastErr = fmt.Errorf("model %q: %w", client.model, err)
	}
	return CompleteResponse{}, fmt.Errorf("robin: all endpoints failed; last error: %w", lastErr)
}

func (r *Robin) Stream(ctx context.Context, req CompleteRequest, fn StreamFunc) (CompleteResponse, error) {
	if fn == nil {
		return CompleteResponse{}, fmt.Errorf("%w: stream function is nil", ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return CompleteResponse{}, err
	}
	if err := req.Validate(); err != nil {
		return CompleteResponse{}, err
	}

	var lastErr error
	for _, client := range r.ordered() {
		var sb strings.Builder
		resp, err := client.Stream(ctx, req, func(delta string) error {
			sb.WriteString(delta)
			return fn(delta)
		})
		if r.logf != nil {
			r.logf(RobinEntry{BaseURL: client.baseURL, Model: client.model}, err)
		}
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return CompleteResponse{}, ctx.Err()
		}
		// A stream that failed mid-flight already emitted deltas; surface the
		// partial failure to the caller instead of restarting on another model.
		if sb.Len() > 0 {
			return CompleteResponse{}, fmt.Errorf("robin: stream from model %q failed mid-flight: %w", client.model, err)
		}
		lastErr = fmt.Errorf("model %q: %w", client.model, err)
	}
	return CompleteResponse{}, fmt.Errorf("robin: all endpoints failed; last error: %w", lastErr)
}

// ParseRobinEndpoints parses "base|key|model;base|key|model" configuration.
// ParseRobinModels splits a comma separated model list, dropping blanks.
func ParseRobinModels(raw string) []string {
	models := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		if model := strings.TrimSpace(part); model != "" {
			models = append(models, model)
		}
	}
	return models
}

// InterleaveRobinEntries flattens provider groups so consecutive requests
// alternate across providers (groq, gemini, groq, gemini, ...) instead of
// draining one provider before moving to the next.
func InterleaveRobinEntries(groups ...[]RobinEntry) []RobinEntry {
	total := 0
	longest := 0
	for _, group := range groups {
		total += len(group)
		if len(group) > longest {
			longest = len(group)
		}
	}
	entries := make([]RobinEntry, 0, total)
	for i := 0; i < longest; i++ {
		for _, group := range groups {
			if i < len(group) {
				entries = append(entries, group[i])
			}
		}
	}
	return entries
}

// BuildRobinGroup expands one provider (base_url, api_key, comma separated
// models) into round-robin entries.
func BuildRobinGroup(baseURL, apiKey, models string) []RobinEntry {
	modelList := ParseRobinModels(models)
	entries := make([]RobinEntry, 0, len(modelList))
	for _, model := range modelList {
		entries = append(entries, RobinEntry{
			BaseURL: strings.TrimSpace(baseURL),
			APIKey:  strings.TrimSpace(apiKey),
			Model:   model,
		})
	}
	return entries
}

func ParseRobinEndpoints(raw string) ([]RobinEntry, error) {
	entries := []RobinEntry{}
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, "|")
		if len(fields) != 3 {
			return nil, fmt.Errorf("invalid robin entry %q: expected base_url|api_key|model", part)
		}
		base := strings.TrimSpace(fields[0])
		key := strings.TrimSpace(fields[1])
		model := strings.TrimSpace(fields[2])
		if base == "" || key == "" || model == "" {
			return nil, fmt.Errorf("invalid robin entry %q: empty field", part)
		}
		entries = append(entries, RobinEntry{BaseURL: base, APIKey: key, Model: model})
	}
	return entries, nil
}
