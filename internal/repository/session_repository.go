package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/alina965/pLaNtS/user-service/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type sessionQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func checkSession(ctx context.Context, db sessionQuerier, userID, sessionID uuid.UUID, lock bool) error {
	query := `SELECT revoked_at IS NOT NULL, expires_at <= clock_timestamp()
		FROM sessions WHERE id = $1 AND user_id = $2`
	if lock {
		query += " FOR UPDATE"
	}
	var revoked, expired bool
	err := db.QueryRow(ctx, query, sessionID, userID).Scan(&revoked, &expired)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrSessionRevoked
	}
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrSessionUnavailable, err)
	}
	if revoked {
		return domain.ErrSessionRevoked
	}
	if expired {
		return domain.ErrSessionExpired
	}
	return nil
}

func (r *RefreshTokenRepository) CheckSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	return checkSession(ctx, r.db, userID, sessionID, false)
}

func (r *RefreshTokenRepository) CreateSession(ctx context.Context, session *domain.Session, token *domain.RefreshToken) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES ($1, $2, $3, $4)`,
		session.ID, session.UserID, session.CreatedAt, session.ExpiresAt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO refresh_tokens (id, token, expires_at, is_revoked, user_id, created_at, session_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		token.ID, token.Token, token.ExpiresAt, token.IsRevoked, token.UserID, token.CreatedAt, token.SessionID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *RefreshTokenRepository) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `UPDATE sessions SET revoked_at = COALESCE(revoked_at, clock_timestamp())
		WHERE id = $1 AND user_id = $2`, sessionID, userID)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrSessionUnavailable, err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrSessionRevoked
	}
	return nil
}
