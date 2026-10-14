package storage

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestMemoryPutGetRoundtrip(t *testing.T) {
	store := NewMemory()
	ctx := context.Background()

	content := []byte("policy document body")
	if err := store.Put(ctx, "key-1", content, "application/pdf"); err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	rc, size, err := store.Get(ctx, "key-1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer rc.Close()

	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestMemoryGetUnknownKey(t *testing.T) {
	store := NewMemory()
	if _, _, err := store.Get(context.Background(), "missing"); err != ErrNotFound {
		t.Errorf("Get(missing) error = %v, want ErrNotFound", err)
	}
}

func TestMemoryPutOverwrites(t *testing.T) {
	store := NewMemory()
	ctx := context.Background()

	if err := store.Put(ctx, "key", []byte("first"), "text/plain"); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	if err := store.Put(ctx, "key", []byte("second"), "text/plain"); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	rc, size, err := store.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	defer rc.Close()
	if size != 6 {
		t.Errorf("size = %d, want 6", size)
	}
}

func TestMemoryPresignUnsupported(t *testing.T) {
	store := NewMemory()
	if _, err := store.PresignGet(context.Background(), "key", time.Minute); err != ErrPresignUnsupported {
		t.Errorf("PresignGet() error = %v, want ErrPresignUnsupported", err)
	}
}

func TestMemoryIsolationBetweenInstances(t *testing.T) {
	a, b := NewMemory(), NewMemory()
	if err := a.Put(context.Background(), "k", []byte("x"), "text/plain"); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	if _, _, err := b.Get(context.Background(), "k"); err != ErrNotFound {
		t.Errorf("separate instances share state: %v", err)
	}
}
