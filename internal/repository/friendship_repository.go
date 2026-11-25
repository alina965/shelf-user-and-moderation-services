package repository

import (
	"context"
	"errors"

	"github.com/alina965/user-service/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FriendshipRepository struct {
	db *pgxpool.Pool
}

func NewFriendshipRepository(db *pgxpool.Pool) *FriendshipRepository {
	return &FriendshipRepository{db: db}
}

func lockFriendPair(ctx context.Context, tx pgx.Tx, first, second uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT id FROM users WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`, first, second)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 2 {
		return domain.ErrProfileNotFound
	}
	return nil
}

func pairState(ctx context.Context, tx pgx.Tx, first, second uuid.UUID) (bool, bool, error) {
	var blocked, friends bool
	err := tx.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM user_blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)),
		EXISTS (SELECT 1 FROM friendships WHERE first_user_id=LEAST($1::uuid,$2::uuid) AND second_user_id=GREATEST($1::uuid,$2::uuid))`, first, second).Scan(&blocked, &friends)
	return blocked, friends, err
}

func (r *FriendshipRepository) Send(ctx context.Context, sender, recipient uuid.UUID) (*domain.FriendRequest, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := lockFriendPair(ctx, tx, sender, recipient); err != nil {
		return nil, err
	}
	blocked, friends, err := pairState(ctx, tx, sender, recipient)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, domain.ErrInteractionBlocked
	}
	if friends {
		return nil, domain.ErrAlreadyFriends
	}
	var existingID, existingSender uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id,sender_id FROM friend_requests
		WHERE LEAST(sender_id,recipient_id)=LEAST($1::uuid,$2::uuid)
		AND GREATEST(sender_id,recipient_id)=GREATEST($1::uuid,$2::uuid) AND status='pending'`, sender, recipient).Scan(&existingID, &existingSender)
	if err == nil {
		return nil, &domain.RequestConflict{ID: existingID, Incoming: existingSender != sender}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	request := &domain.FriendRequest{ID: uuid.New()}
	err = tx.QueryRow(ctx, `INSERT INTO friend_requests (id,sender_id,recipient_id)
		VALUES ($1,$2,$3) RETURNING status,created_at`, request.ID, sender, recipient).Scan(&request.Status, &request.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return request, nil
}

func (r *FriendshipRepository) Resolve(ctx context.Context, actor, requestID uuid.UUID, accept bool) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sender, recipient uuid.UUID
	var status string
	err = tx.QueryRow(ctx, `SELECT sender_id,recipient_id FROM friend_requests WHERE id=$1`, requestID).Scan(&sender, &recipient)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrRequestNotFound
	}
	if err != nil {
		return err
	}
	if actor != sender && actor != recipient {
		return domain.ErrRequestNotFound
	}
	if accept && actor != recipient {
		return domain.ErrRequestRecipient
	}
	if err := lockFriendPair(ctx, tx, sender, recipient); err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT status FROM friend_requests WHERE id=$1 FOR UPDATE`, requestID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrRequestNotFound
	}
	if err != nil {
		return err
	}
	if accept {
		blocked, friends, err := pairState(ctx, tx, sender, recipient)
		if err != nil {
			return err
		}
		if blocked {
			return domain.ErrInteractionBlocked
		}
		if status == "accepted" && friends {
			return tx.Commit(ctx)
		}
		if status != "pending" {
			return domain.ErrRequestState
		}
		_, err = tx.Exec(ctx, `INSERT INTO friendships (first_user_id,second_user_id)
			VALUES (LEAST($1::uuid,$2::uuid),GREATEST($1::uuid,$2::uuid)) ON CONFLICT DO NOTHING`, sender, recipient)
		if err != nil {
			return err
		}
		status = "accepted"
	} else {
		if status == "rejected" || status == "cancelled" {
			return tx.Commit(ctx)
		}
		if status != "pending" {
			return domain.ErrRequestState
		}
		status = "rejected"
		if actor == sender {
			status = "cancelled"
		}
	}
	_, err = tx.Exec(ctx, `UPDATE friend_requests SET status=$2,responded_at=now() WHERE id=$1`, requestID, status)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *FriendshipRepository) Remove(ctx context.Context, actor, friend uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockFriendPair(ctx, tx, actor, friend); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM friendships WHERE first_user_id=LEAST($1::uuid,$2::uuid)
		AND second_user_id=GREATEST($1::uuid,$2::uuid)`, actor, friend)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *FriendshipRepository) SetBlock(ctx context.Context, blocker, blocked uuid.UUID, enabled bool) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockFriendPair(ctx, tx, blocker, blocked); err != nil {
		return err
	}
	if enabled {
		_, err = tx.Exec(ctx, `INSERT INTO user_blocks (blocker_id,blocked_id)
			VALUES ($1,$2) ON CONFLICT DO NOTHING`, blocker, blocked)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE friend_requests SET status='cancelled',responded_at=now()
			WHERE status='pending' AND LEAST(sender_id,recipient_id)=LEAST($1::uuid,$2::uuid)
			AND GREATEST(sender_id,recipient_id)=GREATEST($1::uuid,$2::uuid)`, blocker, blocked)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM user_blocks WHERE blocker_id=$1 AND blocked_id=$2`, blocker, blocked)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *FriendshipRepository) Blocks(ctx context.Context, userID uuid.UUID, limit int, after *domain.BlocksCursor) ([]domain.FriendRow, error) {
	name, id := "", uuid.Nil
	if after != nil {
		name, id = after.Name, after.ID
	}
	rows, err := r.db.Query(ctx, `SELECT u.id,u.username,lower(u.username)
		FROM user_blocks b JOIN users u ON u.id=b.blocked_id
		WHERE b.blocker_id=$1 AND (lower(u.username),u.id)>($2::text,$3::uuid)
		ORDER BY lower(u.username),u.id LIMIT $4`, userID, name, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.FriendRow, 0)
	for rows.Next() {
		var item domain.FriendRow
		if err := rows.Scan(&item.ID, &item.Username, &item.SortName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FriendshipRepository) Requests(ctx context.Context, userID uuid.UUID, direction string, limit int, after *domain.FriendRequestsCursor) ([]domain.FriendRequestItem, error) {
	var afterTime any
	afterID := uuid.Nil
	if after != nil {
		afterTime, afterID = after.CreatedAt, after.ID
	}
	rows, err := r.db.Query(ctx, `SELECT r.id,r.status,r.created_at,u.id,u.username
		FROM friend_requests r JOIN users u ON u.id=CASE WHEN $2='incoming' THEN r.sender_id ELSE r.recipient_id END
		WHERE r.status='pending' AND (($2='incoming' AND r.recipient_id=$1) OR ($2='outgoing' AND r.sender_id=$1))
		AND ($3::timestamptz IS NULL OR (r.created_at,r.id)<($3::timestamptz,$4::uuid))
		ORDER BY r.created_at DESC,r.id DESC LIMIT $5`, userID, direction, afterTime, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.FriendRequestItem, 0)
	for rows.Next() {
		var item domain.FriendRequestItem
		if err := rows.Scan(&item.ID, &item.Status, &item.CreatedAt, &item.User.ID, &item.User.Username); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FriendshipRepository) Friends(ctx context.Context, owner, viewer uuid.UUID, limit int, after *domain.FriendsCursor) ([]domain.FriendRow, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	profile, err := getProfile(ctx, tx, owner, viewer)
	if err != nil {
		return nil, err
	}
	allowed := func(visibility string) bool {
		return owner == viewer || visibility == "public" || (visibility == "friends" && viewer != uuid.Nil && profile.IsFriend && !profile.IsBlocked)
	}
	if !allowed(profile.Privacy.ProfileVisibility) || !allowed(profile.Privacy.FriendsVisibility) {
		return nil, domain.ErrFriendsHidden
	}
	name, id := "", uuid.Nil
	if after != nil {
		name, id = after.Name, after.ID
	}
	rows, err := tx.Query(ctx, `SELECT u.id,u.username,lower(u.username) FROM friendships f
		JOIN users u ON u.id=CASE WHEN f.first_user_id=$1 THEN f.second_user_id ELSE f.first_user_id END
		WHERE (f.first_user_id=$1 OR f.second_user_id=$1) AND (lower(u.username),u.id)>($2::text,$3::uuid)
		ORDER BY lower(u.username),u.id LIMIT $4`, owner, name, id, limit)
	if err != nil {
		return nil, err
	}
	items := make([]domain.FriendRow, 0)
	for rows.Next() {
		var item domain.FriendRow
		if err := rows.Scan(&item.ID, &item.Username, &item.SortName); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}
