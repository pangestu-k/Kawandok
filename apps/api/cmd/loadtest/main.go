package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type result struct {
	status  int
	latency time.Duration
	err     error
}

func main() {
	baseURL := flag.String("url", "http://127.0.0.1:18080", "API base URL")
	total := flag.Int("n", 200, "total number of requests")
	concurrency := flag.Int("c", 10, "concurrent workers")
	email := flag.String("email", "admin@dev.local", "seeded user email for the dev token endpoint")
	path := flag.String("path", "/v1/documents", "authenticated GET path to exercise")
	flag.Parse()

	token, err := issueToken(*baseURL, *email)
	if err != nil {
		fatal("token issuance failed: %v", err)
	}

	results := make([]result, *total)
	jobs := make(chan int, *total)
	var wg sync.WaitGroup
	start := time.Now()

	for worker := 0; worker < *concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 15 * time.Second}
			for idx := range jobs {
				began := time.Now()
				req, err := http.NewRequest(http.MethodGet, *baseURL+*path, nil)
				if err != nil {
					results[idx] = result{err: err}
					continue
				}
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := client.Do(req)
				if err != nil {
					results[idx] = result{err: err}
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				results[idx] = result{status: resp.StatusCode, latency: time.Since(began)}
			}
		}()
	}
	for i := 0; i < *total; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start)

	latencies := make([]float64, 0, len(results))
	statusCounts := map[int]int{}
	failures := 0
	for _, r := range results {
		if r.err != nil {
			failures++
			continue
		}
		latencies = append(latencies, float64(r.latency.Microseconds())/1000.0)
		statusCounts[r.status]++
	}
	sort.Float64s(latencies)

	fmt.Printf("requests=%d concurrency=%d elapsed=%s\n", *total, *concurrency, elapsed.Round(time.Millisecond))
	fmt.Printf("success=%d failures=%d\n", len(latencies), failures)
	for status, count := range statusCounts {
		fmt.Printf("  status %d: %d\n", status, count)
	}
	if len(latencies) > 0 {
		p := func(q float64) float64 {
			idx := int(q * float64(len(latencies)-1))
			return latencies[idx]
		}
		fmt.Printf("latency ms p50=%.2f p95=%.2f p99=%.2f max=%.2f avg=%.2f\n",
			p(0.50), p(0.95), p(0.99), latencies[len(latencies)-1], avg(latencies))
	}
}

func avg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func issueToken(baseURL, email string) (string, error) {
	body := strings.NewReader(fmt.Sprintf(`{"email":%q}`, email))
	resp, err := http.Post(baseURL+"/auth/dev/token", "application/json", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("empty token")
	}
	return parsed.AccessToken, nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "loadtest: "+format+"\n", args...)
	os.Exit(1)
}
