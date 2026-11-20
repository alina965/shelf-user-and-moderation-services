package repository

import (
	"context"
	"errors"

	"github.com/alina965/pLaNtS/user-service/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RefreshTokenRepository struct {
	db *pgxpool.Pool
}

func NewRefreshTokenRepository(db *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) FindRefreshTokenByToken(ctx context.Context, token string) (*domain.RefreshToken, error) {
	row := r.db.QueryRow(ctx, "SELECT id, token, expires_at, is_revoked, user_id, created_at, session_id FROM refresh_tokens WHERE token = $1", token)

	var refreshToken domain.RefreshToken
	err := row.Scan(&refreshToken.ID, &refreshToken.Token, &refreshToken.ExpiresAt, &refreshToken.IsRevoked, &refreshToken.UserID, &refreshToken.CreatedAt, &refreshToken.SessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrRefreshTokenNotFound
	} else if err != nil {
		return nil, err
	}

	return &refreshToken, nil
}

func (r *RefreshTokenRepository) DeleteRefreshTokenById(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, "DELETE FROM refresh_tokens WHERE id = $1", id)

	return err
}

func (r *RefreshTokenRepository) RotateRefreshToken(ctx context.Context, oldToken string, newToken *domain.RefreshToken) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := checkSession(ctx, tx, newToken.UserID, newToken.SessionID, true); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, "DELETE FROM refresh_tokens WHERE token = $1 AND user_id = $2 AND session_id = $3 AND is_revoked = FALSE AND expires_at > clock_timestamp()", oldToken, newToken.UserID, newToken.SessionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInvalidRefreshToken
	}

	_, err = tx.Exec(ctx, "INSERT INTO refresh_tokens (id, token, expires_at, is_revoked, user_id, created_at, session_id) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		newToken.ID,
		newToken.Token,
		newToken.ExpiresAt,
		newToken.IsRevoked,
		newToken.UserID,
		newToken.CreatedAt,
		newToken.SessionID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
