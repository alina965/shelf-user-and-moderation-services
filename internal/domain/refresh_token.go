package domain

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	SessionID uuid.UUID `json:"session_id"`
	ID        uuid.UUID `json:"id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	IsRevoked bool      `json:"is_revoked"`
	UserID    uuid.UUID `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}
