package embedding

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
)

const (
	MockModelID = "mock-embedding-v1"
	maxDim      = 4096
)

type Mock struct {
	dim     int
	modelID string
}

func NewMock(dim int) (*Mock, error) {
	if dim < 1 || dim > maxDim {
		return nil, fmt.Errorf("embedding dim must be between 1 and %d, got %d", maxDim, dim)
	}
	return &Mock{dim: dim, modelID: MockModelID}, nil
}

func (m *Mock) Name() string {
	return m.modelID
}

func (m *Mock) Dim() int {
	return m.dim
}

func (m *Mock) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("embedding: no texts provided")
	}
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vectors[i] = m.vector(text)
	}
	return vectors, nil
}

func (m *Mock) vector(text string) []float32 {
	out := make([]float32, m.dim)
	seed := sha256.Sum256([]byte(m.modelID + "\x00" + text))
	buf := seed
	n := 0
	for n < m.dim {
		for _, b := range buf {
			if n >= m.dim {
				break
			}
			out[n] = float32(int8(b)) / 128.0
			n++
		}
		buf = sha256.Sum256(buf[:])
	}
	return normalize(out)
}

func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return v
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
	return v
}
