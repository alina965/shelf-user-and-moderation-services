package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/alina965/user-service/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InternalRepository struct {
	db *pgxpool.Pool
}

func NewInternalRepository(db *pgxpool.Pool) *InternalRepository {
	return &InternalRepository{db: db}
}

func internalStorageError(err error) error {
	if err == nil || errors.Is(err, domain.ErrSessionRevoked) || errors.Is(err, domain.ErrSessionExpired) || errors.Is(err, domain.ErrSessionUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %w", domain.ErrAccessCheckUnavailable, err)
}

func (r *InternalRepository) snapshot(ctx context.Context, userID, sessionID uuid.UUID) (pgx.Tx, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, internalStorageError(err)
	}
	if userID != uuid.Nil {
		if err := checkSession(ctx, tx, userID, sessionID, false); err != nil {
			tx.Rollback(ctx)
			return nil, err
		}
	}
	return tx, nil
}

func (r *InternalRepository) Introspect(ctx context.Context, userID, sessionID uuid.UUID) (*domain.Identity, error) {
	tx, err := r.snapshot(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var storedID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1`, userID).Scan(&storedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrSessionRevoked
	}
	if err != nil {
		return nil, internalStorageError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, internalStorageError(err)
	}
	return &domain.Identity{Active: true, UserID: &storedID}, nil
}

func (r *InternalRepository) CanReviewCatalogSubmissions(ctx context.Context, userID, sessionID uuid.UUID) (bool, error) {
	tx, err := r.snapshot(ctx, userID, sessionID)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var allowed bool
	err = tx.QueryRow(ctx, `SELECT role='platform_moderator' FROM users WHERE id=$1`, userID).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrSessionRevoked
	}
	if err != nil {
		return false, internalStorageError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, internalStorageError(err)
	}
	return allowed, nil
}

func (r *InternalRepository) AccessChecks(ctx context.Context, viewerID, sessionID uuid.UUID, owners []uuid.UUID) ([]domain.AccessData, error) {
	tx, err := r.snapshot(ctx, viewerID, sessionID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT requested.id,u.id IS NOT NULL,p.user_id IS NOT NULL,
		COALESCE(p.profile_visibility,''),COALESCE(p.description_visibility,''),
		COALESCE(p.friends_visibility,''),COALESCE(p.communities_visibility,''),
		EXISTS (SELECT 1 FROM friendships f WHERE f.first_user_id=LEAST(requested.id,$2::uuid)
			AND f.second_user_id=GREATEST(requested.id,$2::uuid)),
		EXISTS (SELECT 1 FROM user_blocks b WHERE (b.blocker_id=requested.id AND b.blocked_id=$2)
			OR (b.blocker_id=$2 AND b.blocked_id=requested.id))
		FROM unnest($1::uuid[]) AS requested(id)
		LEFT JOIN users u ON u.id=requested.id LEFT JOIN profile_privacy p ON p.user_id=u.id`, owners, viewerID)
	if err != nil {
		return nil, internalStorageError(err)
	}
	defer rows.Close()
	items := make([]domain.AccessData, 0, len(owners))
	for rows.Next() {
		var item domain.AccessData
		err := rows.Scan(&item.OwnerID, &item.Exists, &item.PrivacyPresent,
			&item.Privacy.ProfileVisibility, &item.Privacy.DescriptionVisibility,
			&item.Privacy.FriendsVisibility, &item.Privacy.CommunitiesVisibility, &item.IsFriend, &item.IsBlocked)
		if err != nil {
			return nil, internalStorageError(err)
		}
		if item.Exists && !item.PrivacyPresent {
			return nil, internalStorageError(domain.ErrProfilePrivacyNotFound)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, internalStorageError(err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, internalStorageError(err)
	}
	return items, nil
}

func (r *InternalRepository) Previews(ctx context.Context, ids []uuid.UUID) (*domain.PreviewsResponse, error) {
	rows, err := r.db.Query(ctx, `SELECT requested.id,u.username FROM unnest($1::uuid[]) WITH ORDINALITY AS requested(id,position)
		LEFT JOIN users u ON u.id=requested.id ORDER BY requested.position`, ids)
	if err != nil {
		return nil, internalStorageError(err)
	}
	defer rows.Close()
	response := &domain.PreviewsResponse{Items: make([]domain.UserPreview, 0), NotFoundIDs: make([]uuid.UUID, 0)}
	for rows.Next() {
		var id uuid.UUID
		var username *string
		if err := rows.Scan(&id, &username); err != nil {
			return nil, internalStorageError(err)
		}
		if username == nil {
			response.NotFoundIDs = append(response.NotFoundIDs, id)
		} else {
			response.Items = append(response.Items, domain.UserPreview{ID: id, Username: *username})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, internalStorageError(err)
	}
	return response, nil
}

func (r *InternalRepository) BlockedIDs(ctx context.Context, userID, sessionID, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	tx, err := r.snapshot(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT blocked_id FROM user_blocks WHERE blocker_id=$1 AND blocked_id>$2 ORDER BY blocked_id LIMIT $3`, userID, after, limit)
	if err != nil {
		return nil, internalStorageError(err)
	}
	defer rows.Close()
	items := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, internalStorageError(err)
		}
		items = append(items, id)
	}
	if err := rows.Err(); err != nil {
		return nil, internalStorageError(err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, internalStorageError(err)
	}
	return items, nil
}
