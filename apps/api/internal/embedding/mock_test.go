package embedding

import (
	"context"
	"math"
	"testing"
)

func newTestMock(t *testing.T, dim int) *Mock {
	t.Helper()
	m, err := NewMock(dim)
	if err != nil {
		t.Fatalf("NewMock(%d) error: %v", dim, err)
	}
	return m
}

func TestMockEmbedIsDeterministic(t *testing.T) {
	m := newTestMock(t, 64)
	ctx := context.Background()

	first, err := m.Embed(ctx, []string{"leave policy"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	second, err := m.Embed(ctx, []string{"leave policy"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	for i := range first[0] {
		if first[0][i] != second[0][i] {
			t.Fatalf("vectors differ at index %d", i)
		}
	}
}

func TestMockEmbedDifferentTextsDiffer(t *testing.T) {
	m := newTestMock(t, 64)
	vectors, err := m.Embed(context.Background(), []string{"leave policy", "invoice total"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	same := true
	for i := range vectors[0] {
		if vectors[0][i] != vectors[1][i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different texts produced identical vectors")
	}
}

func TestMockEmbedUnitNorm(t *testing.T) {
	for _, dim := range []int{1, 8, 1536} {
		m := newTestMock(t, dim)
		vectors, err := m.Embed(context.Background(), []string{"norm check"})
		if err != nil {
			t.Fatalf("Embed() error: %v", err)
		}
		var sum float64
		for _, x := range vectors[0] {
			sum += float64(x) * float64(x)
		}
		if math.Abs(sum-1.0) > 1e-5 {
			t.Errorf("dim %d: norm^2 = %f, want ~1", dim, sum)
		}
	}
}

func TestMockEmbedBatchOrder(t *testing.T) {
	m := newTestMock(t, 32)
	single, err := m.Embed(context.Background(), []string{"second"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	batch, err := m.Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed() error: %v", err)
	}
	for i := range single[0] {
		if single[0][i] != batch[1][i] {
			t.Fatal("batch embedding does not match single embedding")
		}
	}
}

func TestMockEmbedValidation(t *testing.T) {
	if _, err := NewMock(0); err == nil {
		t.Error("dim 0 must be rejected")
	}
	if _, err := NewMock(maxDim + 1); err == nil {
		t.Error("dim above max must be rejected")
	}

	m := newTestMock(t, 8)
	if _, err := m.Embed(context.Background(), nil); err == nil {
		t.Error("empty batch must be rejected")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Embed(ctx, []string{"text"}); err == nil {
		t.Error("cancelled context must be rejected")
	}
}

func TestMockMetadata(t *testing.T) {
	m := newTestMock(t, 128)
	if m.Dim() != 128 {
		t.Errorf("Dim() = %d, want 128", m.Dim())
	}
	if m.Name() != MockModelID {
		t.Errorf("Name() = %q, want %q", m.Name(), MockModelID)
	}
}
