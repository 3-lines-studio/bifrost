package storage

import (
	"context"
	"io"

	"github.com/3-lines-studio/bifrost/example/modular/internal/config"
)

// Module is a leaf module. It owns the object-storage handle. The scaffold uses
// a minimal interface so it compiles without an S3 SDK; swap Module for a real
// adapter in production.
type Module struct {
	bucket string
	region string
}

func New() *Module { return &Module{} }

func (m *Module) Wire(cfg *config.Module) {
	storage := cfg.Value().Storage
	m.bucket = storage.Bucket
	m.region = storage.Region
}

func (m *Module) Put(ctx context.Context, key string, reader io.Reader) error {
	// Stub. Replace with the S3 SDK PutObject call in production.
	return nil
}

func (m *Module) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	// Stub. Replace with the S3 SDK GetObject call in production.
	return nil, nil
}
