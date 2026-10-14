package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"office-ai-agent/apps/api/internal/audit"
	"office-ai-agent/apps/api/internal/auth"
	"office-ai-agent/apps/api/internal/authz"
	"office-ai-agent/apps/api/internal/chat"
	"office-ai-agent/apps/api/internal/config"
	"office-ai-agent/apps/api/internal/devseed"
	"office-ai-agent/apps/api/internal/documents"
	"office-ai-agent/apps/api/internal/embedding"
	"office-ai-agent/apps/api/internal/finance"
	"office-ai-agent/apps/api/internal/httpapi"
	"office-ai-agent/apps/api/internal/llm"
	"office-ai-agent/apps/api/internal/logging"
	"office-ai-agent/apps/api/internal/queue"
	"office-ai-agent/apps/api/internal/retrieval"
	"office-ai-agent/apps/api/internal/storage"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	log := logging.New(cfg.Env, os.Getenv("API_LOG_LEVEL"))

	if cfg.Env == config.EnvProd {
		return errors.New("no production authentication provider is configured yet (planned for M5); refusing to start with the dev auth fixture")
	}

	var pinger httpapi.Pinger
	var db *sql.DB
	if cfg.DatabaseURL != "" {
		db, err = sql.Open("postgres", cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
		db.SetConnMaxLifetime(5 * time.Minute)
		db.SetMaxOpenConns(10)
		pinger = db

		if cfg.DevSeed {
			if err := devseed.Run(context.Background(), db); err != nil {
				return fmt.Errorf("dev seed: %w", err)
			}
			log.Info("dev seed applied", "env", cfg.Env)
		}
	}

	store, err := buildStorage(context.Background(), cfg, log)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	api := httpapi.New(cfg, log, pinger)
	api.Register(mux)

	var handler http.Handler = mux
	if db != nil {
		dispatcher := buildDispatcher(cfg, log)
		repo := documents.NewRepo(db)
		checker := authz.NewChecker(db)
		auditLog := audit.NewLogger(db)
		svc := documents.NewService(
			repo,
			store,
			checker,
			auditLog,
			db,
			dispatcher,
			cfg.MaxUploadBytes,
			time.Duration(cfg.PresignTTLSeconds)*time.Second,
			cfg.StorageBucket,
			log,
		)
		documents.Register(mux, &documents.Handlers{Svc: svc, Log: log})

		var embedProvider embedding.Provider
		if cfg.EmbeddingProvider == config.ProviderMock {
			embedProvider, err = embedding.NewMock(cfg.EmbeddingDim)
			if err != nil {
				return err
			}
		}

		llmProvider, err := buildLLMProvider(cfg)
		if err != nil {
			return err
		}

		financeSvc := finance.NewService(db, auditLog, log)
		chatSvc := chat.NewService(chat.NewRepo(db), retrieval.NewSearcher(db, embedProvider, cfg.RetrievalTopK), llmProvider, chat.NewAuthorizer(checker), financeSvc, log)
		chat.Register(mux, &chat.Handlers{Svc: chatSvc})
		handler = auth.Middleware(auth.NewDev(db), mux)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		go sweeperLoop(ctx, svc, log)
		httpServer := &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           httpapi.Chain(log, handler),
			ReadHeaderTimeout: 5 * time.Second,
		}
		return serve(ctx, httpServer, log, cfg)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.Chain(log, handler),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Warn("database not configured; starting in health-only mode")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, httpServer, log, cfg)
}

func buildLLMProvider(cfg config.Config) (llm.Provider, error) {
	switch cfg.LLMProvider {
	case config.ProviderOpenAI:
		return llm.NewOpenAI(llm.OpenAIConfig{
			BaseURL: cfg.LLMBaseURL,
			APIKey:  cfg.LLMAPIKey,
			Model:   cfg.LLMModel,
		})
	default:
		return llm.NewGroundedMock(), nil
	}
}

func ensureBucketWithRetry(ctx context.Context, store *storage.S3, log *slog.Logger) error {
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if err := store.EnsureBucket(ctx); err != nil {
			lastErr = err
			log.Warn("bucket ensure attempt failed; retrying", "attempt", attempt, "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
			continue
		}
		return nil
	}
	return lastErr
}

func buildDispatcher(cfg config.Config, log *slog.Logger) queue.Dispatcher {
	if cfg.RedisAddr == "" {
		log.Warn("redis not configured; ingestion jobs wait for the outbox sweep of another instance")
		return queue.Nop{}
	}
	d, err := queue.NewRedis(cfg.RedisAddr, queue.DefaultStream)
	if err != nil {
		log.Warn("redis unavailable at startup; jobs wait for the outbox sweep", "error", err)
		return queue.Nop{}
	}
	return d
}

func sweeperLoop(ctx context.Context, svc *documents.Service, log *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	if n, err := svc.SweepPending(ctx); err != nil {
		log.Warn("startup outbox sweep failed", "error", err)
	} else if n > 0 {
		log.Info("startup outbox sweep dispatched jobs", "count", n)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := svc.SweepPending(ctx)
			if err != nil {
				log.Warn("outbox sweep failed", "error", err)
				continue
			}
			if n > 0 {
				log.Info("outbox sweep dispatched jobs", "count", n)
			}
		}
	}
}

func buildStorage(ctx context.Context, cfg config.Config, log *slog.Logger) (storage.Storage, error) {
	switch cfg.StorageBackend {
	case config.StorageMemory:
		log.Warn("using in-memory object storage; uploaded files are lost on restart")
		return storage.NewMemory(), nil
	case config.StorageS3:
		s3Store, err := storage.NewS3(context.Background(), storage.S3Config{
			EndpointURL: cfg.R2Endpoint(),
			Region:      "auto",
			Bucket:      cfg.StorageBucket,
			AccessKeyID: cfg.R2AccessKeyID,
			SecretKey:   cfg.R2SecretAccessKey,
		})
		if err != nil {
			return nil, err
		}
		bucketCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := ensureBucketWithRetry(bucketCtx, s3Store, log); err != nil {
			log.Warn("could not verify or create the storage bucket; uploads may fail",
				"bucket", cfg.StorageBucket, "error", err)
		}
		return s3Store, nil
	default:
		return nil, fmt.Errorf("unknown storage backend %q", cfg.StorageBackend)
	}
}

func serve(ctx context.Context, httpServer *http.Server, log *slog.Logger, cfg config.Config) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "port", cfg.Port, "env", cfg.Env,
			"llm_provider", cfg.LLMProvider, "storage_backend", cfg.StorageBackend)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
