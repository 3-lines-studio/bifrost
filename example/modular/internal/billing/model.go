package billing

import "time"

// Invoice is the aggregate root for the billing domain.
type Invoice struct {
	ID        string
	UserID    string
	Amount    int64
	Status    string
	CreatedAt time.Time
}
