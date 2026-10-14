package storage

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"
)

type Memory struct {
	mu      sync.RWMutex
	objects map[string]memoryObject
}

type memoryObject struct {
	data        []byte
	contentType string
}

func NewMemory() *Memory {
	return &Memory{objects: make(map[string]memoryObject)}
}

func (m *Memory) Put(_ context.Context, key string, data []byte, contentType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = memoryObject{data: bytes.Clone(data), contentType: contentType}
	return nil
}

func (m *Memory) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	m.mu.RLock()
	obj, ok := m.objects[key]
	m.mu.RUnlock()
	if !ok {
		return nil, 0, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(obj.data)), int64(len(obj.data)), nil
}

func (m *Memory) PresignGet(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", ErrPresignUnsupported
}
