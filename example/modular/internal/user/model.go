package user

import "time"

// User is the aggregate root for the user domain.
type User struct {
	ID        string
	Email     string
	Plan      string
	CreatedAt time.Time
}
