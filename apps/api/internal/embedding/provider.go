package embedding

import "context"

type Provider interface {
	Name() string
	Dim() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
