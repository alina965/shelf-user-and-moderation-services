package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alina965/user-service/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LoginAttemptsRepository struct {
	db *pgxpool.Pool
}

func NewLoginAttemptsRepository(db *pgxpool.Pool) *LoginAttemptsRepository {
	return &LoginAttemptsRepository{db: db}
}

func loginCheckError(err error) error {
	if err == nil || errors.Is(err, domain.ErrLoginLocked) || errors.Is(err, domain.ErrAccessCheckUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %w", domain.ErrAccessCheckUnavailable, err)
}

func (r *LoginAttemptsRepository) FindByIdentifier(ctx context.Context, identifier string) (*domain.LoginAttempt, error) {
	row := r.db.QueryRow(ctx, "SELECT identifier, failed_count, last_failed_at, locked_until FROM login_attempts WHERE identifier = $1", identifier)
	attempt := &domain.LoginAttempt{}
	err := row.Scan(&attempt.Identifier, &attempt.FailedCount, &attempt.LastFailedAt, &attempt.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.LoginAttemptsNotFound
	} else if err != nil {
		return nil, err
	}

	return attempt, nil
}

func (r *LoginAttemptsRepository) CheckLocked(ctx context.Context, identifier string) error {
	var locked bool
	err := r.db.QueryRow(ctx, `SELECT locked_until > clock_timestamp() FROM login_attempts
		WHERE identifier = $1 AND locked_until IS NOT NULL`, identifier).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return loginCheckError(err)
	}
	if locked {
		return domain.ErrLoginLocked
	}
	return nil
}

func lockLoginAttempt(ctx context.Context, tx pgx.Tx, identifier string) (*domain.LoginAttempt, time.Time, error) {
	_, err := tx.Exec(ctx, `INSERT INTO login_attempts (identifier) VALUES ($1) ON CONFLICT (identifier) DO NOTHING`, identifier)
	if err != nil {
		return nil, time.Time{}, loginCheckError(err)
	}
	attempt := &domain.LoginAttempt{Identifier: identifier}
	err = tx.QueryRow(ctx, `SELECT failed_count, last_failed_at, locked_until FROM login_attempts
		WHERE identifier = $1 FOR UPDATE`, identifier).Scan(&attempt.FailedCount, &attempt.LastFailedAt, &attempt.LockedUntil)
	if err != nil {
		return nil, time.Time{}, loginCheckError(err)
	}
	var now time.Time
	err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now)
	return attempt, now, loginCheckError(err)
}

func (r *LoginAttemptsRepository) RecordFailure(ctx context.Context, identifier string) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, loginCheckError(err)
	}
	defer tx.Rollback(ctx)
	attempt, now, err := lockLoginAttempt(ctx, tx, identifier)
	if err != nil {
		return false, loginCheckError(err)
	}
	if attempt.LockedUntil != nil && attempt.LockedUntil.After(now) {
		return true, nil
	}
	if attempt.LockedUntil != nil || attempt.LastFailedAt == nil || !attempt.LastFailedAt.Add(time.Hour).After(now) {
		attempt.FailedCount = 0
	}
	attempt.FailedCount++
	var lockedUntil *time.Time
	if attempt.FailedCount >= 5 {
		lockedUntil = new(now.Add(time.Hour))
	}
	_, err = tx.Exec(ctx, `UPDATE login_attempts SET failed_count = $2, last_failed_at = $3, locked_until = $4
		WHERE identifier = $1`, identifier, attempt.FailedCount, now, lockedUntil)
	if err != nil {
		return false, loginCheckError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, loginCheckError(err)
	}
	return lockedUntil != nil, nil
}

func resetLoginAttempts(ctx context.Context, tx pgx.Tx, identifier string) error {
	attempt, now, err := lockLoginAttempt(ctx, tx, identifier)
	if err != nil {
		return err
	}
	if attempt.LockedUntil != nil && attempt.LockedUntil.After(now) {
		return domain.ErrLoginLocked
	}
	_, err = tx.Exec(ctx, `UPDATE login_attempts SET failed_count = 0, last_failed_at = NULL, locked_until = NULL WHERE identifier = $1`, identifier)
	return loginCheckError(err)
}
