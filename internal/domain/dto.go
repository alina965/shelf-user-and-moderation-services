package domain

import (
	"time"

	"github.com/google/uuid"
)

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type RegisterRequest struct {
	Email                string `json:"email"`
	Username             string `json:"username"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
}

type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type RegisterResponse struct {
	UserID uuid.UUID `json:"user_id"`
}

type LoginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         UserResponse `json:"user"`
}

type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type UserResponse struct {
	ID           uuid.UUID      `json:"id"`
	Email        string         `json:"email"`
	Username     string         `json:"username"`
	AvatarURL    *string        `json:"avatar_url"`
	BannerURL    *string        `json:"banner_url"`
	Description  *string        `json:"description"`
	FriendsCount int            `json:"friends_count"`
	Privacy      ProfilePrivacy `json:"privacy"`
}

type UserPreview struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	AvatarURL *string   `json:"avatar_url"`
}

type UserSearchResponse struct {
	Items      []UserPreview `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

type UserSearchCursor struct {
	Version int       `json:"v"`
	Query   string    `json:"q"`
	Rank    int       `json:"rank"`
	Name    string    `json:"name"`
	ID      uuid.UUID `json:"id"`
}

type UserSearchRow struct {
	UserPreview
	Rank     int
	SortName string
}

type PublicProfile struct {
	UserPreview
	BannerURL    *string `json:"banner_url"`
	IsPrivate    bool    `json:"is_private"`
	Description  *string `json:"description,omitempty"`
	FriendsCount *int    `json:"friends_count,omitempty"`
}

type ProfileData struct {
	ID           uuid.UUID
	Username     string
	Description  *string
	Privacy      ProfilePrivacy
	IsFriend     bool
	IsBlocked    bool
	FriendsCount int
}
