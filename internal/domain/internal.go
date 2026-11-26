package domain

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrInternalInput     = errors.New("invalid internal request")
	ErrInternalToken     = errors.New("invalid user access token")
	ErrInternalForbidden = errors.New("internal operation forbidden")
)

type Identity struct {
	Active bool       `json:"active"`
	UserID *uuid.UUID `json:"user_id,omitempty"`
}

type IntrospectRequest struct {
	AccessToken string `json:"access_token"`
}

const PermissionCatalogSubmissionsReview = "catalog_submissions.review"

type PermissionCheckRequest struct {
	AccessToken string `json:"access_token"`
	Permission  string `json:"permission"`
}

type PermissionCheckResponse struct {
	Allowed bool `json:"allowed"`
}

type AccessCheck struct {
	OwnerID    uuid.UUID `json:"owner_id"`
	Section    string    `json:"section"`
	Visibility string    `json:"visibility,omitempty"`
}

type AccessChecksRequest struct {
	AccessToken string        `json:"access_token"`
	Checks      []AccessCheck `json:"checks"`
}

type AccessDecision struct {
	OwnerID   uuid.UUID `json:"owner_id"`
	Section   string    `json:"section"`
	Exists    bool      `json:"exists"`
	Allowed   bool      `json:"allowed"`
	IsFriend  bool      `json:"is_friend"`
	IsBlocked bool      `json:"is_blocked"`
}

type AccessChecksResponse struct {
	Items []AccessDecision `json:"items"`
}

type AccessData struct {
	OwnerID        uuid.UUID
	Exists         bool
	PrivacyPresent bool
	Privacy        ProfilePrivacy
	IsFriend       bool
	IsBlocked      bool
}

type PreviewsRequest struct {
	UserIDs []uuid.UUID `json:"user_ids"`
}

type PreviewsResponse struct {
	Items       []UserPreview `json:"items"`
	NotFoundIDs []uuid.UUID   `json:"not_found_ids"`
}

type BlockedIDsResponse struct {
	Items      []uuid.UUID `json:"items"`
	NextCursor *string     `json:"next_cursor"`
}

type BlockedIDsCursor struct {
	Version int       `json:"v"`
	UserID  uuid.UUID `json:"user_id"`
	ID      uuid.UUID `json:"id"`
}
