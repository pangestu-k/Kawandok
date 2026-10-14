package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	EnvelopeVersion = 1
	StreamField     = "envelope"
	DefaultStream   = "office-ai:ingest:v1"
	JobTypeIngest   = "ingest_document"
	JobTypeReindex  = "reindex_document"
)

type JobMessage struct {
	JobID             string
	JobType           string
	IdempotencyKey    string
	DocumentID        string
	DocumentVersionID string
	VersionNumber     int
	StorageBucket     string
	StorageKey        string
}

func (m JobMessage) Envelope() ([]byte, error) {
	if m.IdempotencyKey == "" {
		m.IdempotencyKey = m.JobID
	}
	return json.Marshal(map[string]any{
		"job_id":           m.JobID,
		"job_type":         m.JobType,
		"envelope_version": EnvelopeVersion,
		"idempotency_key":  m.IdempotencyKey,
		"payload": map[string]any{
			"document_id":         m.DocumentID,
			"document_version_id": m.DocumentVersionID,
			"version_number":      m.VersionNumber,
			"storage_bucket":      m.StorageBucket,
			"storage_key":         m.StorageKey,
		},
	})
}

type Dispatcher interface {
	Dispatch(ctx context.Context, msg JobMessage) error
	Close() error
}

type Nop struct{}

func (Nop) Dispatch(context.Context, JobMessage) error { return nil }

func (Nop) Close() error { return nil }

type Redis struct {
	client *redis.Client
	stream string
}

func NewRedis(addr, stream string) (*Redis, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 3 * time.Second,
		ReadTimeout: 3 * time.Second,
		MaxRetries:  2,
		Protocol:    2,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("queue: redis ping: %w", err)
	}
	return &Redis{client: client, stream: stream}, nil
}

func (r *Redis) Dispatch(ctx context.Context, msg JobMessage) error {
	envelope, err := msg.Envelope()
	if err != nil {
		return fmt.Errorf("queue: marshal envelope: %w", err)
	}
	if err := r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.stream,
		Values: map[string]any{StreamField: string(envelope)},
	}).Err(); err != nil {
		return fmt.Errorf("queue: xadd: %w", err)
	}
	return nil
}

func (r *Redis) DeadLetter(ctx context.Context, raw []byte, reason string) error {
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.stream + ":dead",
		Values: map[string]any{"reason": reason, StreamField: string(raw)},
	}).Err()
}

func (r *Redis) Close() error {
	return r.client.Close()
}
