package billing

import "context"

// repo is the narrow persistence port. It is unexported because it is consumed
// only by this module; its single implementation lives in store.go.
type repo interface {
	Create(ctx context.Context, invoice *Invoice) error
	Get(ctx context.Context, id string) (*Invoice, error)
	UpdateStatus(ctx context.Context, id, status string) error
}
