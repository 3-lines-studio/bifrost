package user

import "context"

// repo is the narrow persistence port. It is unexported because it is consumed
// only by this module; its single implementation lives in store.go.
type repo interface {
	Get(ctx context.Context, id string) (*User, error)
	List(ctx context.Context) ([]User, error)
}
