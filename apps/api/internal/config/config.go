package config

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	EnvDev  = "dev"
	EnvProd = "prod"

	ProviderMock = "mock"

	StorageMemory = "memory"
	StorageS3     = "s3"

	DefaultPort              = "8080"
	DefaultEmbeddingDim      = 1536
	DefaultPresignTTLSeconds = 60
	DefaultMaxUploadBytes    = 26_214_400
	maxEmbeddingDim          = 10000
	maxPresignTTLSeconds     = 900
	maxUploadBytes           = 104_857_600
)

type Config struct {
	Env               string
	Port              string
	DatabaseURL       string
	RedisAddr         string
	LLMProvider       string
	EmbeddingProvider string
	EmbeddingDim      int

	StorageBackend    string
	StorageBucket     string
	S3Endpoint        string
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	PresignTTLSeconds int
	MaxUploadBytes    int64
	DevSeed           bool
}

type Getter func(key string) string

func Load(getenv Getter) (Config, error) {
	cfg := Config{
		Env:               strings.TrimSpace(getenv("API_ENV")),
		Port:              strings.TrimSpace(getenv("API_PORT")),
		DatabaseURL:       strings.TrimSpace(getenv("API_DATABASE_URL")),
		RedisAddr:         strings.TrimSpace(getenv("API_REDIS_ADDR")),
		LLMProvider:       strings.TrimSpace(getenv("API_LLM_PROVIDER")),
		EmbeddingProvider: strings.TrimSpace(getenv("API_EMBEDDING_PROVIDER")),

		StorageBackend:    strings.ToLower(strings.TrimSpace(getenv("API_STORAGE_BACKEND"))),
		StorageBucket:     strings.TrimSpace(getenv("API_STORAGE_BUCKET")),
		S3Endpoint:        strings.TrimSpace(getenv("API_S3_ENDPOINT")),
		R2AccountID:       strings.TrimSpace(getenv("API_R2_ACCOUNT_ID")),
		R2AccessKeyID:     strings.TrimSpace(getenv("API_R2_ACCESS_KEY_ID")),
		R2SecretAccessKey: strings.TrimSpace(getenv("API_R2_SECRET_ACCESS_KEY")),
	}

	if raw := strings.TrimSpace(getenv("API_EMBEDDING_DIM")); raw == "" {
		cfg.EmbeddingDim = DefaultEmbeddingDim
	} else if parsed, err := strconv.Atoi(raw); err != nil {
		return Config{}, fmt.Errorf("API_EMBEDDING_DIM: %w", err)
	} else {
		cfg.EmbeddingDim = parsed
	}

	if raw := strings.TrimSpace(getenv("API_PRESIGN_TTL_SECONDS")); raw == "" {
		cfg.PresignTTLSeconds = DefaultPresignTTLSeconds
	} else if parsed, err := strconv.Atoi(raw); err != nil {
		return Config{}, fmt.Errorf("API_PRESIGN_TTL_SECONDS: %w", err)
	} else {
		cfg.PresignTTLSeconds = parsed
	}

	if raw := strings.TrimSpace(getenv("API_MAX_UPLOAD_BYTES")); raw == "" {
		cfg.MaxUploadBytes = DefaultMaxUploadBytes
	} else if parsed, err := strconv.ParseInt(raw, 10, 64); err != nil {
		return Config{}, fmt.Errorf("API_MAX_UPLOAD_BYTES: %w", err)
	} else {
		cfg.MaxUploadBytes = parsed
	}

	seedRaw := strings.ToLower(strings.TrimSpace(getenv("API_DEV_SEED")))
	seedExplicit := seedRaw != ""
	switch seedRaw {
	case "", "1", "true", "yes":
		cfg.DevSeed = true
	default:
		cfg.DevSeed = false
	}

	applyDefaults(&cfg, seedExplicit)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config, seedExplicit bool) {
	if cfg.Env == "" {
		cfg.Env = EnvDev
	}
	if cfg.Port == "" {
		cfg.Port = DefaultPort
	}
	if cfg.LLMProvider == "" {
		cfg.LLMProvider = ProviderMock
	}
	if cfg.EmbeddingProvider == "" {
		cfg.EmbeddingProvider = ProviderMock
	}
	if cfg.StorageBackend == "" {
		if cfg.Env == EnvProd {
			cfg.StorageBackend = StorageS3
		} else {
			cfg.StorageBackend = StorageMemory
		}
	}
	if !seedExplicit {
		cfg.DevSeed = cfg.Env == EnvDev
	}
}

func (c Config) Validate() error {
	switch c.Env {
	case EnvDev, EnvProd:
	default:
		return fmt.Errorf("API_ENV must be %q or %q, got %q", EnvDev, EnvProd, c.Env)
	}

	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("API_PORT must be an integer between 1 and 65535, got %q", c.Port)
	}

	if c.EmbeddingDim < 1 || c.EmbeddingDim > maxEmbeddingDim {
		return fmt.Errorf("API_EMBEDDING_DIM must be between 1 and %d, got %d", maxEmbeddingDim, c.EmbeddingDim)
	}

	if c.LLMProvider != ProviderMock {
		return fmt.Errorf("API_LLM_PROVIDER %q is not available yet; supported: %q", c.LLMProvider, ProviderMock)
	}
	if c.EmbeddingProvider != ProviderMock {
		return fmt.Errorf("API_EMBEDDING_PROVIDER %q is not available yet; supported: %q", c.EmbeddingProvider, ProviderMock)
	}

	switch c.StorageBackend {
	case StorageMemory:
		if c.Env == EnvProd {
			return fmt.Errorf("production requires the %q storage backend", StorageS3)
		}
	case StorageS3:
		if c.StorageBucket == "" {
			return fmt.Errorf("API_STORAGE_BUCKET is required when the storage backend is %q", StorageS3)
		}
		if c.R2AccessKeyID == "" || c.R2SecretAccessKey == "" {
			return fmt.Errorf("API_R2_ACCESS_KEY_ID and API_R2_SECRET_ACCESS_KEY are required when the storage backend is %q", StorageS3)
		}
		if c.R2Endpoint() == "" {
			return fmt.Errorf("either API_S3_ENDPOINT or API_R2_ACCOUNT_ID is required when the storage backend is %q", StorageS3)
		}
	default:
		return fmt.Errorf("API_STORAGE_BACKEND must be %q or %q, got %q", StorageMemory, StorageS3, c.StorageBackend)
	}

	if c.PresignTTLSeconds < 1 || c.PresignTTLSeconds > maxPresignTTLSeconds {
		return fmt.Errorf("API_PRESIGN_TTL_SECONDS must be between 1 and %d, got %d", maxPresignTTLSeconds, c.PresignTTLSeconds)
	}
	if c.MaxUploadBytes < 1 || c.MaxUploadBytes > maxUploadBytes {
		return fmt.Errorf("API_MAX_UPLOAD_BYTES must be between 1 and %d, got %d", maxUploadBytes, c.MaxUploadBytes)
	}
	if c.DevSeed && c.Env != EnvDev {
		return fmt.Errorf("dev seeding is only allowed when API_ENV is %q", EnvDev)
	}
	return nil
}

func (c Config) R2Endpoint() string {
	if c.S3Endpoint != "" {
		return c.S3Endpoint
	}
	if c.R2AccountID != "" {
		return "https://" + c.R2AccountID + ".r2.cloudflarestorage.com"
	}
	return ""
}
