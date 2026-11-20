package repository

import (
	"context"
	"errors"

	"github.com/alina965/pLaNtS/user-service/internal/domain"
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
