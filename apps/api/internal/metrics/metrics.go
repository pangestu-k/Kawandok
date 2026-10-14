package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type counterVec struct {
	mu     sync.Mutex
	values map[string]*int64
	help   string
	name   string
}

func newCounterVec(name, help string) *counterVec {
	return &counterVec{name: name, help: help, values: map[string]*int64{}}
}

func (c *counterVec) inc(label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.values[label]
	if !ok {
		v = new(int64)
		c.values[label] = v
	}
	atomic.AddInt64(v, 1)
}

func (c *counterVec) add(label string, delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.values[label]
	if !ok {
		v = new(int64)
		c.values[label] = v
	}
	atomic.AddInt64(v, delta)
}

func (c *counterVec) render() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var sb strings.Builder
	fmt.Fprintf(&sb, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
	labels := make([]string, 0, len(c.values))
	for k := range c.values {
		labels = append(labels, k)
	}
	sort.Strings(labels)
	for _, k := range labels {
		if k == "" {
			continue
		}
		fmt.Fprintf(&sb, "%s{%s} %d\n", c.name, k, atomic.LoadInt64(c.values[k]))
	}
	return sb.String()
}

var (
	httpRequestsTotal = newCounterVec("office_ai_http_requests_total", "HTTP requests processed")
	llmPromptTokens   = newCounterVec("office_ai_llm_prompt_tokens_total", "LLM prompt tokens consumed")
	llmOutputTokens   = newCounterVec("office_ai_llm_output_tokens_total", "LLM output tokens produced")
)

type histogram struct {
	mu      sync.Mutex
	buckets []float64
	counts  []int64
	sum     float64
	count   int64
}

var httpDuration = &histogram{
	buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
	counts:  make([]int64, 11),
}

func (h *histogram) observe(ms float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum += ms
	h.count++
	for i, b := range h.buckets {
		if ms <= b {
			h.counts[i]++
		}
	}
}

func (h *histogram) render() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var sb strings.Builder
	sb.WriteString("# HELP office_ai_http_request_duration_ms HTTP request duration in milliseconds\n")
	sb.WriteString("# TYPE office_ai_http_request_duration_ms histogram\n")
	for i, b := range h.buckets {
		fmt.Fprintf(&sb, "office_ai_http_request_duration_ms_bucket{le=\"%g\"} %d\n", b, h.counts[i])
	}
	fmt.Fprintf(&sb, "office_ai_http_request_duration_ms_sum %g\n", h.sum)
	fmt.Fprintf(&sb, "office_ai_http_request_duration_ms_count %d\n", h.count)
	return sb.String()
}

func ObserveHTTP(route string, method string, status int, durationMS float64) {
	httpRequestsTotal.inc(fmt.Sprintf("route=%q method=%q status=%q", route, method, strconvI(status)))
	httpDuration.observe(durationMS)
}

func strconvI(v int) string {
	return fmt.Sprintf("%d", v)
}

func ObserveLLMTokens(provider string, prompt, output int64) {
	if prompt > 0 {
		llmPromptTokens.add(fmt.Sprintf("provider=%q", provider), prompt)
	}
	if output > 0 {
		llmOutputTokens.add(fmt.Sprintf("provider=%q", provider), output)
	}
}

func Render() string {
	var sb strings.Builder
	sb.WriteString(httpRequestsTotal.render())
	sb.WriteString(httpDuration.render())
	sb.WriteString(llmPromptTokens.render())
	sb.WriteString(llmOutputTokens.render())
	return sb.String()
}
