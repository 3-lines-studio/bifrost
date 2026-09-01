package notify

import (
	"context"

	"github.com/hibiken/asynq"

	"github.com/3-lines-studio/bifrost/example/modular/internal/web"
)

// RegisterTasks mounts the module's async task handlers.
func (m *Module) RegisterTasks(tasks *asynq.ServeMux) {
	tasks.HandleFunc("notify:email", m.handleEmail)
}

func (m *Module) handleEmail(ctx context.Context, task *asynq.Task) error {
	var payload emailPayload
	if err := web.DecodeTask(task.Payload(), &payload); err != nil {
		return err
	}
	return m.Deliver(ctx, payload.To, payload.Subject)
}

// Run returns a nil-returning no-op for modules with no background loop.
func (m *Module) Run(ctx context.Context) error {
	return nil
}
