package config

import (
	"strings"
	"testing"
)

func fullValidEnv() map[string]string {
	return map[string]string{
		"API_ENV":                  "prod",
		"API_PORT":                 "9090",
		"API_DATABASE_URL":         "postgres://localhost/db",
		"API_REDIS_ADDR":           "127.0.0.1:6380",
		"API_LLM_PROVIDER":         "mock",
		"API_EMBEDDING_PROVIDER":   "mock",
		"API_EMBEDDING_DIM":        "768",
		"API_STORAGE_BACKEND":      "s3",
		"API_STORAGE_BUCKET":       "docs",
		"API_S3_ENDPOINT":          "https://acct.r2.cloudflarestorage.com",
		"API_R2_ACCESS_KEY_ID":     "key",
		"API_R2_SECRET_ACCESS_KEY": "secret",
		"API_PRESIGN_TTL_SECONDS":  "60",
		"API_MAX_UPLOAD_BYTES":     "1048576",
		"API_DEV_SEED":             "false",
		"API_JWT_SIGNING_KEY":      "01234567890123456789012345678901",
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "dev defaults applied when env is empty",
			env:  map[string]string{"API_JWT_SIGNING_KEY": "01234567890123456789012345678901"},
			want: Config{
				Env:                "dev",
				Port:               "8080",
				LLMProvider:        "mock",
				EmbeddingProvider:  "mock",
				EmbeddingDim:       1536,
				StorageBackend:     "memory",
				PresignTTLSeconds:  60,
				MaxUploadBytes:     26214400,
				DevSeed:            true,
				RetrievalTopK:      6,
				RateLimitPerMinute: 120,
				MetricsEnabled:     true,
				JWTSigningKey:      "01234567890123456789012345678901",
				JWTIssuer:          "office-ai-dev",
				JWTAudience:        "office-ai-api",
				JWTTTLMinutes:      60,
			},
		},
		{
			name: "explicit production values are kept",
			env:  fullValidEnv(),
			want: Config{
				Env:                "prod",
				Port:               "9090",
				DatabaseURL:        "postgres://localhost/db",
				RedisAddr:          "127.0.0.1:6380",
				LLMProvider:        "mock",
				EmbeddingProvider:  "mock",
				EmbeddingDim:       768,
				StorageBackend:     "s3",
				StorageBucket:      "docs",
				S3Endpoint:         "https://acct.r2.cloudflarestorage.com",
				R2AccessKeyID:      "key",
				R2SecretAccessKey:  "secret",
				PresignTTLSeconds:  60,
				MaxUploadBytes:     1048576,
				RetrievalTopK:      6,
				RateLimitPerMinute: 120,
				MetricsEnabled:     true,
				JWTIssuer:          "office-ai-dev",
				JWTAudience:        "office-ai-api",
				JWTSigningKey:      "01234567890123456789012345678901",
				JWTTTLMinutes:      60,
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
			name:    "unknown llm provider rejected",
			env:     map[string]string{"API_LLM_PROVIDER": "openai"},
			wantErr: true,
		},
		{
			name:    "openai embedding without configuration rejected",
			env:     map[string]string{"API_EMBEDDING_PROVIDER": "openai"},
			wantErr: true,
		},
		{
			name: "openai embedding configuration is accepted",
			env: with(fullValidEnv(), func(e map[string]string) {
				e["API_EMBEDDING_PROVIDER"] = "openai"
				e["API_EMBEDDING_BASE_URL"] = "https://provider.example/v1"
				e["API_EMBEDDING_MODEL"] = "test-model"
				e["API_EMBEDDING_API_KEY"] = "test-key"
			}),
			want: withCfg(Config{
				Env: "prod", Port: "9090", DatabaseURL: "postgres://localhost/db", RedisAddr: "127.0.0.1:6380",
				LLMProvider: "mock", EmbeddingProvider: "openai", EmbeddingDim: 768,
				EmbeddingBaseURL: "https://provider.example/v1", EmbeddingModel: "test-model", EmbeddingAPIKey: "test-key",
				StorageBackend: "s3", StorageBucket: "docs", S3Endpoint: "https://acct.r2.cloudflarestorage.com", R2AccessKeyID: "key", R2SecretAccessKey: "secret",
				PresignTTLSeconds: 60, MaxUploadBytes: 1048576, RetrievalTopK: 6, RateLimitPerMinute: 120, MetricsEnabled: true,
				JWTIssuer: "office-ai-dev", JWTAudience: "office-ai-api", JWTSigningKey: "01234567890123456789012345678901", JWTTTLMinutes: 60,
			}),
		},
		{
			name:    "invalid storage backend rejected",
			env:     map[string]string{"API_STORAGE_BACKEND": "google-drive"},
			wantErr: true,
		},
		{
			name: "prod with memory backend rejected",
			env: with(fullValidEnv(), func(e map[string]string) {
				e["API_STORAGE_BACKEND"] = "memory"
			}),
			wantErr: true,
		},
		{
			name: "s3 backend without bucket rejected",
			env: with(fullValidEnv(), func(e map[string]string) {
				delete(e, "API_STORAGE_BUCKET")
			}),
			wantErr: true,
		},
		{
			name: "s3 backend without credentials rejected",
			env: with(fullValidEnv(), func(e map[string]string) {
				delete(e, "API_R2_SECRET_ACCESS_KEY")
			}),
			wantErr: true,
		},
		{
			name: "s3 backend without endpoint or account rejected",
			env: with(fullValidEnv(), func(e map[string]string) {
				delete(e, "API_S3_ENDPOINT")
				delete(e, "API_R2_ACCOUNT_ID")
			}),
			wantErr: true,
		},
		{
			name: "s3 backend with r2 account builds endpoint",
			env: with(fullValidEnv(), func(e map[string]string) {
				delete(e, "API_S3_ENDPOINT")
				e["API_R2_ACCOUNT_ID"] = "acct42"
			}),
			want: withCfg(Config{
				Env:                "prod",
				Port:               "9090",
				DatabaseURL:        "postgres://localhost/db",
				RedisAddr:          "127.0.0.1:6380",
				LLMProvider:        "mock",
				EmbeddingProvider:  "mock",
				EmbeddingDim:       768,
				StorageBackend:     "s3",
				StorageBucket:      "docs",
				R2AccountID:        "acct42",
				R2AccessKeyID:      "key",
				R2SecretAccessKey:  "secret",
				PresignTTLSeconds:  60,
				MaxUploadBytes:     1048576,
				RetrievalTopK:      6,
				RateLimitPerMinute: 120,
				MetricsEnabled:     true,
				JWTIssuer:          "office-ai-dev",
				JWTAudience:        "office-ai-api",
				JWTSigningKey:      "01234567890123456789012345678901",
				JWTTTLMinutes:      60,
			}),
		},
		{
			name: "presign ttl out of range rejected",
			env: with(fullValidEnv(), func(e map[string]string) {
				e["API_PRESIGN_TTL_SECONDS"] = "3600"
			}),
			wantErr: true,
		},
		{
			name: "max upload out of range rejected",
			env: with(fullValidEnv(), func(e map[string]string) {
				e["API_MAX_UPLOAD_BYTES"] = "0"
			}),
			wantErr: true,
		},
		{
			name: "dev seed in prod rejected",
			env: with(fullValidEnv(), func(e map[string]string) {
				e["API_DEV_SEED"] = "true"
			}),
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
				t.Errorf("Load() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestR2Endpoint(t *testing.T) {
	cfg := Config{S3Endpoint: "https://override", R2AccountID: "acct"}
	if got := cfg.R2Endpoint(); got != "https://override" {
		t.Errorf("R2Endpoint() = %q, want explicit endpoint", got)
	}
	cfg.S3Endpoint = ""
	if got := cfg.R2Endpoint(); got != "https://acct.r2.cloudflarestorage.com" {
		t.Errorf("R2Endpoint() = %q, want derived r2 endpoint", got)
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

func with(base map[string]string, mutate func(map[string]string)) map[string]string {
	clone := make(map[string]string, len(base)+2)
	for k, v := range base {
		clone[k] = v
	}
	mutate(clone)
	return clone
}

func withCfg(c Config) Config {
	return c
}
