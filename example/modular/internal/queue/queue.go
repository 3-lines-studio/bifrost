package queue

import (
	"context"

	"github.com/hibiken/asynq"

	"github.com/3-lines-studio/bifrost/example/modular/internal/config"
)

// Module is a leaf module. It owns the asynq client used to enqueue tasks. Other
// modules depend on it and read Client. The asynq server that consumes tasks
// belongs to the app composition root.
type Module struct {
	client *asynq.Client
}

func New() *Module { return &Module{} }

func (m *Module) Wire(cfg *config.Module) {
	redis := cfg.Value().Redis
	m.client = asynq.NewClient(asynq.RedisClientOpt{
		Addr:     redis.Addr,
		Password: redis.Password,
		DB:       redis.DB,
	})
}

func (m *Module) Client() *asynq.Client { return m.client }

// Enqueue sends a task. A nil ctx uses context.Background().
func (m *Module) Enqueue(ctx context.Context, typename string, payload []byte, opts ...asynq.Option) error {
	task := asynq.NewTask(typename, payload, opts...)
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := m.client.EnqueueContext(ctx, task)
	return err
}
