package interservice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/alina965/user-service/internal/domain"
	"github.com/google/uuid"
)

type TokenParser interface {
	Parse(string) (string, string, error)
}

type Repository interface {
	Introspect(context.Context, uuid.UUID, uuid.UUID) (*domain.Identity, error)
	CanReviewCatalogSubmissions(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	AccessChecks(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) ([]domain.AccessData, error)
	Previews(context.Context, []uuid.UUID) (*domain.PreviewsResponse, error)
	BlockedIDs(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int) ([]uuid.UUID, error)
}

type Service struct {
	repository Repository
	parser     TokenParser
}

func NewService(repository Repository, parser TokenParser) *Service {
	return &Service{repository: repository, parser: parser}
}

func (s *Service) identity(token string) (uuid.UUID, uuid.UUID, error) {
	if token == "" {
		return uuid.Nil, uuid.Nil, domain.ErrInternalToken
	}
	user, session, err := s.parser.Parse(token)
	if err != nil {
		return uuid.Nil, uuid.Nil, domain.ErrInternalToken
	}
	userID, userErr := uuid.Parse(user)
	sessionID, sessionErr := uuid.Parse(session)
	if userErr != nil || sessionErr != nil || userID == uuid.Nil || sessionID == uuid.Nil {
		return uuid.Nil, uuid.Nil, domain.ErrInternalToken
	}
	return userID, sessionID, nil
}

func (s *Service) Introspect(ctx context.Context, token string) (*domain.Identity, error) {
	userID, sessionID, err := s.identity(token)
	if err != nil {
		return &domain.Identity{Active: false}, nil
	}
	identity, err := s.repository.Introspect(ctx, userID, sessionID)
	if errors.Is(err, domain.ErrSessionRevoked) || errors.Is(err, domain.ErrSessionExpired) {
		return &domain.Identity{Active: false}, nil
	}
	return identity, err
}

func (s *Service) CheckPermission(ctx context.Context, request domain.PermissionCheckRequest) (*domain.PermissionCheckResponse, error) {
	if request.Permission != domain.PermissionCatalogSubmissionsReview {
		return nil, domain.ErrInternalInput
	}
	userID, sessionID, err := s.identity(request.AccessToken)
	if err != nil {
		return nil, err
	}
	allowed, err := s.repository.CanReviewCatalogSubmissions(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	return &domain.PermissionCheckResponse{Allowed: allowed}, nil
}

func (s *Service) AccessChecks(ctx context.Context, request domain.AccessChecksRequest) (*domain.AccessChecksResponse, error) {
	if len(request.Checks) < 1 || len(request.Checks) > 100 {
		return nil, domain.ErrInternalInput
	}
	owners := make([]uuid.UUID, 0, len(request.Checks))
	seen := make(map[uuid.UUID]bool)
	for _, check := range request.Checks {
		if check.OwnerID == uuid.Nil {
			return nil, domain.ErrInternalInput
		}
		switch check.Section {
		case "profile", "description", "friends", "communities":
			if check.Visibility != "" {
				return nil, domain.ErrInternalInput
			}
		case "collection":
			if check.Visibility != "public" && check.Visibility != "friends" && check.Visibility != "private" {
				return nil, domain.ErrInternalInput
			}
		default:
			return nil, domain.ErrInternalInput
		}
		if !seen[check.OwnerID] {
			owners = append(owners, check.OwnerID)
			seen[check.OwnerID] = true
		}
	}
	viewerID, sessionID := uuid.Nil, uuid.Nil
	if request.AccessToken != "" {
		var err error
		viewerID, sessionID, err = s.identity(request.AccessToken)
		if err != nil {
			return nil, err
		}
	}
	data, err := s.repository.AccessChecks(ctx, viewerID, sessionID, owners)
	if err != nil {
		return nil, err
	}
	byOwner := make(map[uuid.UUID]domain.AccessData, len(data))
	for _, item := range data {
		byOwner[item.OwnerID] = item
	}
	response := &domain.AccessChecksResponse{Items: make([]domain.AccessDecision, 0, len(request.Checks))}
	for _, check := range request.Checks {
		profile, ok := byOwner[check.OwnerID]
		if !ok {
			return nil, domain.ErrAccessCheckUnavailable
		}
		canSee := func(visibility string) bool {
			return profile.Exists && (viewerID == check.OwnerID || visibility == "public" ||
				(visibility == "friends" && viewerID != uuid.Nil && profile.IsFriend && !profile.IsBlocked))
		}
		visibility := profile.Privacy.ProfileVisibility
		switch check.Section {
		case "description":
			visibility = profile.Privacy.DescriptionVisibility
		case "friends":
			visibility = profile.Privacy.FriendsVisibility
		case "communities":
			visibility = profile.Privacy.CommunitiesVisibility
		case "collection":
			visibility = check.Visibility
		}
		response.Items = append(response.Items, domain.AccessDecision{OwnerID: check.OwnerID, Section: check.Section,
			Exists: profile.Exists, Allowed: canSee(profile.Privacy.ProfileVisibility) && canSee(visibility),
			IsFriend: profile.IsFriend, IsBlocked: profile.IsBlocked})
	}
	return response, nil
}

func (s *Service) Previews(ctx context.Context, ids []uuid.UUID) (*domain.PreviewsResponse, error) {
	if len(ids) < 1 || len(ids) > 100 {
		return nil, domain.ErrInternalInput
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			return nil, domain.ErrInternalInput
		}
		seen[id] = true
	}
	return s.repository.Previews(ctx, ids)
}

func (s *Service) BlockedIDs(ctx context.Context, token string, owner uuid.UUID, limit int, cursor string) (*domain.BlockedIDsResponse, error) {
	viewerID, sessionID, err := s.identity(token)
	if err != nil {
		return nil, err
	}
	if owner != viewerID {
		return nil, domain.ErrInternalForbidden
	}
	if limit < 1 || limit > 20 {
		return nil, domain.ErrInvalidLimit
	}
	after := uuid.Nil
	if cursor != "" {
		if len(cursor) > 4096 {
			return nil, domain.ErrInvalidCursor
		}
		data, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, domain.ErrInvalidCursor
		}
		var value domain.BlockedIDsCursor
		if err := json.Unmarshal(data, &value); err != nil || value.Version != 1 || value.UserID != viewerID || value.ID == uuid.Nil {
			return nil, domain.ErrInvalidCursor
		}
		after = value.ID
	}
	items, err := s.repository.BlockedIDs(ctx, viewerID, sessionID, after, limit+1)
	if err != nil {
		return nil, err
	}
	response := &domain.BlockedIDsResponse{Items: items}
	if len(items) > limit {
		data, err := json.Marshal(domain.BlockedIDsCursor{Version: 1, UserID: viewerID, ID: items[limit-1]})
		if err != nil {
			return nil, err
		}
		next := base64.RawURLEncoding.EncodeToString(data)
		response.NextCursor = &next
		response.Items = items[:limit]
	}
	return response, nil
}
