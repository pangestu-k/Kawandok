package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

const redactedPlaceholder = "[REDACTED]"

var sensitiveKeys = map[string]struct{}{
	"api_key":       {},
	"apikey":        {},
	"authorization": {},
	"body":          {},
	"chunk":         {},
	"completion":    {},
	"content":       {},
	"cookie":        {},
	"document_text": {},
	"password":      {},
	"prompt":        {},
	"secret":        {},
	"set_cookie":    {},
	"token":         {},
}

func New(env, level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(writerForEnv(env), &slog.HandlerOptions{Level: parseLevel(level)}))
}

func writerForEnv(env string) io.Writer {
	return os.Stdout
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func IsSensitive(key string) bool {
	_, ok := sensitiveKeys[normalizeKey(key)]
	return ok
}

func Redact(value string) string {
	return redactedPlaceholder
}

func Scrub(fields map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		if IsSensitive(k) {
			out[k] = redactedPlaceholder
			continue
		}
		out[k] = scrubValue(v)
	}
	return out
}

func scrubValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		return Scrub(typed)
	case []any:
		scrubbed := make([]any, len(typed))
		for i, item := range typed {
			scrubbed[i] = scrubValue(item)
		}
		return scrubbed
	default:
		return v
	}
}
