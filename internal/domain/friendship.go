package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSelfFriendship     = errors.New("cannot act on yourself")
	ErrInteractionBlocked = errors.New("interaction blocked")
	ErrAlreadyFriends     = errors.New("already friends")
	ErrRequestNotFound    = errors.New("friend request not found")
	ErrRequestState       = errors.New("friend request is no longer pending")
	ErrRequestRecipient   = errors.New("only recipient may accept request")
	ErrInvalidDirection   = errors.New("invalid friend request direction")
	ErrFriendsHidden      = errors.New("friends list is private")
)

type RequestConflict struct {
	ID       uuid.UUID
	Incoming bool
}

func (e *RequestConflict) Error() string {
	return "pending friend request already exists"
}

type FriendRequest struct {
	ID        uuid.UUID `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type FriendRequestItem struct {
	FriendRequest
	User UserPreview `json:"user"`
}

type FriendRequestsResponse struct {
	Items      []FriendRequestItem `json:"items"`
	NextCursor *string             `json:"next_cursor"`
}

type FriendRequestsCursor struct {
	Version   int       `json:"v"`
	UserID    uuid.UUID `json:"user_id"`
	Direction string    `json:"direction"`
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

type FriendsCursor struct {
	Version  int       `json:"v"`
	OwnerID  uuid.UUID `json:"owner_id"`
	ViewerID uuid.UUID `json:"viewer_id"`
	Name     string    `json:"name"`
	ID       uuid.UUID `json:"id"`
}

type FriendRow struct {
	UserPreview
	SortName string
}
