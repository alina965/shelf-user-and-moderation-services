package domain

import "time"

type LoginAttempt struct {
	Identifier   string
	FailedCount  int
	LastFailedAt *time.Time
	LockedUntil  *time.Time
}
