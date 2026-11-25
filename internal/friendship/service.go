package friendship

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alina965/user-service/internal/domain"
	"github.com/google/uuid"
)

type Repository interface {
	Send(context.Context, uuid.UUID, uuid.UUID) (*domain.FriendRequest, error)
	Resolve(context.Context, uuid.UUID, uuid.UUID, bool) error
	Remove(context.Context, uuid.UUID, uuid.UUID) error
	Requests(context.Context, uuid.UUID, string, int, *domain.FriendRequestsCursor) ([]domain.FriendRequestItem, error)
	Friends(context.Context, uuid.UUID, uuid.UUID, int, *domain.FriendsCursor) ([]domain.FriendRow, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func storageError(err error) error {
	if err == nil {
		return nil
	}
	var conflict *domain.RequestConflict
	if errors.As(err, &conflict) {
		return err
	}
	for _, expected := range []error{domain.ErrProfileNotFound, domain.ErrRequestNotFound, domain.ErrRequestRecipient,
		domain.ErrRequestState, domain.ErrInteractionBlocked, domain.ErrAlreadyFriends, domain.ErrFriendsHidden} {
		if errors.Is(err, expected) {
			return err
		}
	}
	return fmt.Errorf("%w: %w", domain.ErrAccessCheckUnavailable, err)
}

func (s *Service) Send(ctx context.Context, sender, recipient uuid.UUID) (*domain.FriendRequest, error) {
	if sender == recipient {
		return nil, domain.ErrSelfFriendship
	}
	request, err := s.repository.Send(ctx, sender, recipient)
	return request, storageError(err)
}

func (s *Service) Resolve(ctx context.Context, actor, requestID uuid.UUID, accept bool) error {
	return storageError(s.repository.Resolve(ctx, actor, requestID, accept))
}

func (s *Service) Remove(ctx context.Context, actor, friend uuid.UUID) error {
	if actor == friend {
		return domain.ErrSelfFriendship
	}
	return storageError(s.repository.Remove(ctx, actor, friend))
}

func decodeCursor(cursor string, value any) error {
	if len(cursor) > 4096 {
		return domain.ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return domain.ErrInvalidCursor
	}
	if err := json.Unmarshal(data, value); err != nil {
		return domain.ErrInvalidCursor
	}
	return nil
}

func encodeCursor(value any) (*string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	cursor := base64.RawURLEncoding.EncodeToString(data)
	return &cursor, nil
}

func (s *Service) Requests(ctx context.Context, userID uuid.UUID, direction string, limit int, cursor string) (*domain.FriendRequestsResponse, error) {
	if direction != "incoming" && direction != "outgoing" {
		return nil, domain.ErrInvalidDirection
	}
	if limit < 1 || limit > 20 {
		return nil, domain.ErrInvalidLimit
	}
	var after *domain.FriendRequestsCursor
	if cursor != "" {
		after = &domain.FriendRequestsCursor{}
		if err := decodeCursor(cursor, after); err != nil {
			return nil, err
		}
		if after.Version != 1 || after.UserID != userID || after.Direction != direction || after.ID == uuid.Nil || after.CreatedAt.IsZero() {
			return nil, domain.ErrInvalidCursor
		}
	}
	items, err := s.repository.Requests(ctx, userID, direction, limit+1, after)
	if err != nil {
		return nil, storageError(err)
	}
	response := &domain.FriendRequestsResponse{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		response.NextCursor, err = encodeCursor(domain.FriendRequestsCursor{Version: 1, UserID: userID, Direction: direction, CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, err
		}
		response.Items = items[:limit]
	}
	return response, nil
}

func (s *Service) Friends(ctx context.Context, owner, viewer uuid.UUID, limit int, cursor string) (*domain.UserSearchResponse, error) {
	if limit < 1 || limit > 20 {
		return nil, domain.ErrInvalidLimit
	}
	var after *domain.FriendsCursor
	if cursor != "" {
		after = &domain.FriendsCursor{}
		if err := decodeCursor(cursor, after); err != nil {
			return nil, err
		}
		if after.Version != 1 || after.OwnerID != owner || after.ViewerID != viewer || after.ID == uuid.Nil || after.Name == "" {
			return nil, domain.ErrInvalidCursor
		}
	}
	rows, err := s.repository.Friends(ctx, owner, viewer, limit+1, after)
	if err != nil {
		return nil, storageError(err)
	}
	response := &domain.UserSearchResponse{Items: make([]domain.UserPreview, 0)}
	if len(rows) > limit {
		last := rows[limit-1]
		response.NextCursor, err = encodeCursor(domain.FriendsCursor{Version: 1, OwnerID: owner, ViewerID: viewer, Name: last.SortName, ID: last.ID})
		if err != nil {
			return nil, err
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		response.Items = append(response.Items, row.UserPreview)
	}
	return response, nil
}
