package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alina965/user-service/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user *domain.User) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "INSERT INTO users (id, username, email, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)",
		user.ID, user.Username, user.Email, user.PasswordHash, user.CreatedAt)

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_email_key",
			"users_email_ci_unique",
			"users_username_ci_unique":
			return domain.ErrAccountExists
		}
	}

	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, "INSERT INTO profile_privacy (user_id) VALUES ($1)", user.ID)

	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *UserRepository) GetUserById(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row := r.db.QueryRow(ctx, "SELECT id, username, email, password_hash, created_at FROM users WHERE id = $1", id)
	user := &domain.User{}
	err := row.Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) GetUserByIdentifier(ctx context.Context, identifier string) (*domain.User, error) {
	row := r.db.QueryRow(ctx, "SELECT id, email, username, password_hash, created_at FROM users WHERE lower(email) = lower($1) OR lower(username) = lower($1)", identifier)
	user := &domain.User{}
	err := row.Scan(&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) GetProfilePrivacy(ctx context.Context, id uuid.UUID) (*domain.ProfilePrivacy, error) {
	row := r.db.QueryRow(ctx, "SELECT profile_visibility, description_visibility, friends_visibility, communities_visibility FROM profile_privacy WHERE user_id = $1", id)
	privacy := &domain.ProfilePrivacy{}

	err := row.Scan(&privacy.ProfileVisibility, &privacy.DescriptionVisibility, &privacy.FriendsVisibility, &privacy.CommunitiesVisibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProfilePrivacyNotFound
	} else if err != nil {
		return nil, err
	}

	return privacy, nil
}

func (r *UserRepository) SearchUsers(ctx context.Context, query string, limit int, after *domain.UserSearchCursor) ([]domain.UserSearchRow, error) {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
	rank, name, id := -1, "", uuid.Nil
	if after != nil {
		rank, name, id = after.Rank, after.Name, after.ID
	}
	rows, err := r.db.Query(ctx, `WITH matches AS (
		SELECT id, username, lower(username) AS sort_name,
			CASE WHEN lower(username) = $1 THEN 0
			WHEN lower(username) LIKE $2 ESCAPE '\' THEN 1 ELSE 2 END AS match_rank
		FROM users WHERE lower(username) LIKE $3 ESCAPE '\'
	)
	SELECT id, username, sort_name, match_rank FROM matches
	WHERE (match_rank, sort_name, id) > ($4::int, $5::text, $6::uuid)
	ORDER BY match_rank, sort_name, id LIMIT $7`,
		query, escaped+"%", "%"+escaped+"%", rank, name, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.UserSearchRow, 0)
	for rows.Next() {
		var item domain.UserSearchRow
		if err := rows.Scan(&item.ID, &item.Username, &item.SortName, &item.Rank); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *UserRepository) GetProfile(ctx context.Context, ownerID, viewerID uuid.UUID) (*domain.ProfileData, error) {
	return getProfile(ctx, r.db, ownerID, viewerID)
}

func getProfile(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ownerID, viewerID uuid.UUID) (*domain.ProfileData, error) {
	profile := &domain.ProfileData{}
	err := db.QueryRow(ctx, `SELECT u.id, u.username, u.description,
		p.profile_visibility, p.description_visibility, p.friends_visibility, p.communities_visibility,
		EXISTS (SELECT 1 FROM friendships WHERE first_user_id = LEAST($1::uuid, $2::uuid)
			AND second_user_id = GREATEST($1::uuid, $2::uuid)),
		EXISTS (SELECT 1 FROM user_blocks WHERE (blocker_id = $1 AND blocked_id = $2)
			OR (blocker_id = $2 AND blocked_id = $1)),
		(SELECT count(*) FROM friendships WHERE first_user_id = $1 OR second_user_id = $1)
		FROM users u LEFT JOIN profile_privacy p ON p.user_id = u.id WHERE u.id = $1`, ownerID, viewerID).Scan(
		&profile.ID, &profile.Username, &profile.Description,
		&profile.Privacy.ProfileVisibility, &profile.Privacy.DescriptionVisibility,
		&profile.Privacy.FriendsVisibility, &profile.Privacy.CommunitiesVisibility,
		&profile.IsFriend, &profile.IsBlocked, &profile.FriendsCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrAccessCheckUnavailable, err)
	}
	return profile, nil
}
