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

	DefaultPort         = "8080"
	DefaultEmbeddingDim = 1536
	maxEmbeddingDim     = 10000
)

type Config struct {
	Env               string
	Port              string
	DatabaseURL       string
	RedisAddr         string
	LLMProvider       string
	EmbeddingProvider string
	EmbeddingDim      int
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
	}

	dim := strings.TrimSpace(getenv("API_EMBEDDING_DIM"))
	if dim == "" {
		cfg.EmbeddingDim = DefaultEmbeddingDim
	} else {
		parsed, err := strconv.Atoi(dim)
		if err != nil {
			return Config{}, fmt.Errorf("API_EMBEDDING_DIM: %w", err)
		}
		cfg.EmbeddingDim = parsed
	}

	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
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
		return fmt.Errorf("API_LLM_PROVIDER %q is not available in M0; supported: %q", c.LLMProvider, ProviderMock)
	}
	if c.EmbeddingProvider != ProviderMock {
		return fmt.Errorf("API_EMBEDDING_PROVIDER %q is not available in M0; supported: %q", c.EmbeddingProvider, ProviderMock)
	}
	return nil
}
