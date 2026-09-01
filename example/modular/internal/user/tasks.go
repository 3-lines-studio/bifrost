package user

import (
	"context"

	"github.com/hibiken/asynq"

	"github.com/3-lines-studio/bifrost/example/modular/internal/web"
)

// RegisterTasks mounts the module's async task handlers.
func (m *Module) RegisterTasks(tasks *asynq.ServeMux) {
	tasks.HandleFunc("user:activate", m.handleActivate)
}

func (m *Module) handleActivate(ctx context.Context, task *asynq.Task) error {
	var payload activationPayload
	if err := web.DecodeTask(task.Payload(), &payload); err != nil {
		return err
	}
	return m.Activate(ctx, payload.UserID)
}

type activationPayload struct {
	UserID string `json:"userId"`
}

// Run returns a nil-returning no-op for modules with no background loop. It is
// present to keep the module surface uniform.
func (m *Module) Run(ctx context.Context) error {
	return nil
}
