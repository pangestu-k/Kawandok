package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "defaults applied when env is empty",
			env:  map[string]string{},
			want: Config{
				Env:               "dev",
				Port:              "8080",
				LLMProvider:       "mock",
				EmbeddingProvider: "mock",
				EmbeddingDim:      1536,
			},
		},
		{
			name: "explicit values are kept",
			env: map[string]string{
				"API_ENV":                "prod",
				"API_PORT":               "9090",
				"API_DATABASE_URL":       "postgres://localhost/db",
				"API_REDIS_ADDR":         "127.0.0.1:6380",
				"API_LLM_PROVIDER":       "mock",
				"API_EMBEDDING_PROVIDER": "mock",
				"API_EMBEDDING_DIM":      "768",
			},
			want: Config{
				Env:               "prod",
				Port:              "9090",
				DatabaseURL:       "postgres://localhost/db",
				RedisAddr:         "127.0.0.1:6380",
				LLMProvider:       "mock",
				EmbeddingProvider: "mock",
				EmbeddingDim:      768,
			},
		},
		{
			name:    "invalid env rejected",
			env:     map[string]string{"API_ENV": "staging"},
			wantErr: true,
		},
		{
			name:    "invalid port rejected",
			env:     map[string]string{"API_PORT": "not-a-port"},
			wantErr: true,
		},
		{
			name:    "out of range port rejected",
			env:     map[string]string{"API_PORT": "70000"},
			wantErr: true,
		},
		{
			name:    "invalid embedding dim rejected",
			env:     map[string]string{"API_EMBEDDING_DIM": "zero"},
			wantErr: true,
		},
		{
			name:    "zero embedding dim rejected",
			env:     map[string]string{"API_EMBEDDING_DIM": "0"},
			wantErr: true,
		},
		{
			name:    "unknown llm provider rejected",
			env:     map[string]string{"API_LLM_PROVIDER": "openai"},
			wantErr: true,
		},
		{
			name:    "unknown embedding provider rejected",
			env:     map[string]string{"API_EMBEDDING_PROVIDER": "openai"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getter := func(key string) string { return tt.env[key] }
			got, err := Load(getter)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadInvalidDimErrorMentionsVariable(t *testing.T) {
	_, err := Load(func(key string) string {
		if key == "API_EMBEDDING_DIM" {
			return "abc"
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected error for non-integer embedding dim")
	}
	if !strings.Contains(err.Error(), "API_EMBEDDING_DIM") {
		t.Errorf("error %q does not mention API_EMBEDDING_DIM", err)
	}
}
