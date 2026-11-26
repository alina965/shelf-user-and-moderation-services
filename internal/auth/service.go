package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alina965/user-service/internal/domain"

	"github.com/google/uuid"
)

type Service struct {
	hasher                  PasswordHasher
	jwtService              TokenService
	userRepository          UserRepository
	refreshTokenRepository  RefreshTokenRepository
	refreshTokenTTL         time.Duration
	loginAttemptsRepository LoginAttemptsRepository
}

func NewAuthService(hasher PasswordHasher, jwtService TokenService, refreshTokenRepository RefreshTokenRepository, userRepository UserRepository, loginAttemptsRepository LoginAttemptsRepository, refreshTokenTTL time.Duration) *Service {
	return &Service{
		hasher:                  hasher,
		jwtService:              jwtService,
		refreshTokenRepository:  refreshTokenRepository,
		userRepository:          userRepository,
		refreshTokenTTL:         refreshTokenTTL,
		loginAttemptsRepository: loginAttemptsRepository,
	}
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash string, password string) bool
}

type TokenService interface {
	Generate(userID string, email string, sessionID string) (*domain.TokenPair, error)
}

type RefreshTokenRepository interface {
	CreateSession(ctx context.Context, session *domain.Session, token *domain.RefreshToken, identifier string) error
	CheckSession(ctx context.Context, userID, sessionID uuid.UUID) error
	RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error
	FindRefreshTokenByToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	RotateRefreshToken(ctx context.Context, oldToken string, newToken *domain.RefreshToken) error
}

type LoginAttemptsRepository interface {
	CheckLocked(ctx context.Context, identifier string) error
	RecordFailure(ctx context.Context, identifier string) (bool, error)
}

type UserRepository interface {
	GetProfile(ctx context.Context, ownerID, viewerID uuid.UUID) (*domain.ProfileData, error)
	SearchUsers(ctx context.Context, query string, limit int, after *domain.UserSearchCursor) ([]domain.UserSearchRow, error)
	CreateUser(ctx context.Context, user *domain.User) error
	RegistrationConflicts(ctx context.Context, email, username string) (map[string]string, error)
	GetUserById(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetUserByIdentifier(ctx context.Context, identifier string) (*domain.User, error)
	GetProfilePrivacy(ctx context.Context, id uuid.UUID) (*domain.ProfilePrivacy, error)
}

func (s *Service) GetProfile(ctx context.Context, ownerID, viewerID uuid.UUID) (*domain.PublicProfile, error) {
	profile, err := s.userRepository.GetProfile(ctx, ownerID, viewerID)
	if err != nil {
		return nil, err
	}
	isOwner := viewerID != uuid.Nil && viewerID == ownerID
	canSee := func(visibility string) bool {
		return isOwner || visibility == "public" || (visibility == "friends" && viewerID != uuid.Nil && profile.IsFriend && !profile.IsBlocked)
	}
	response := &domain.PublicProfile{
		UserPreview: domain.UserPreview{ID: profile.ID, Username: profile.Username},
		IsPrivate:   !canSee(profile.Privacy.ProfileVisibility),
	}
	if response.IsPrivate {
		return response, nil
	}
	if canSee(profile.Privacy.DescriptionVisibility) {
		response.Description = profile.Description
	}
	if canSee(profile.Privacy.FriendsVisibility) {
		response.FriendsCount = &profile.FriendsCount
	}
	return response, nil
}

func (s *Service) SearchUsers(ctx context.Context, query string, limit int, cursor string) (*domain.UserSearchResponse, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, domain.ErrQueryRequired
	}
	if utf8.RuneCountInString(query) < 2 {
		return nil, domain.ErrQueryTooShort
	}
	if limit < 1 || limit > 20 {
		return nil, domain.ErrInvalidLimit
	}
	var after *domain.UserSearchCursor
	if cursor != "" {
		if len(cursor) > 4096 {
			return nil, domain.ErrInvalidCursor
		}
		data, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, domain.ErrInvalidCursor
		}
		after = &domain.UserSearchCursor{}
		if err := json.Unmarshal(data, after); err != nil || after.Version != 1 || after.Query != query ||
			after.Rank < 0 || after.Rank > 2 || after.Name == "" || after.ID == uuid.Nil {
			return nil, domain.ErrInvalidCursor
		}
	}
	rows, err := s.userRepository.SearchUsers(ctx, query, limit+1, after)
	if err != nil {
		return nil, err
	}
	response := &domain.UserSearchResponse{Items: make([]domain.UserPreview, 0)}
	if len(rows) > limit {
		last := rows[limit-1]
		data, err := json.Marshal(domain.UserSearchCursor{Version: 1, Query: query, Rank: last.Rank, Name: last.SortName, ID: last.ID})
		if err != nil {
			return nil, err
		}
		next := base64.RawURLEncoding.EncodeToString(data)
		response.NextCursor = &next
		rows = rows[:limit]
	}
	for _, row := range rows {
		response.Items = append(response.Items, row.UserPreview)
	}
	return response, nil
}

func (s *Service) Register(ctx context.Context, email, password, passwordConfirmation, username string) (*domain.RegisterResponse, error) {
	email = strings.TrimSpace(email)
	username = strings.TrimSpace(username)

	fields := registrationErrors(email, username, password, passwordConfirmation)
	invalid := len(fields) > 0
	checkEmail, checkUsername := "", ""
	if fields["email"] == "" {
		checkEmail = email
	}
	if fields["username"] == "" {
		checkUsername = username
	}
	if checkEmail != "" || checkUsername != "" {
		conflicts, err := s.userRepository.RegistrationConflicts(ctx, checkEmail, checkUsername)
		if err != nil {
			return nil, err
		}
		for field, message := range conflicts {
			fields[field] = message
		}
	}
	if len(fields) > 0 {
		cause := domain.ErrAccountExists
		if invalid {
			cause = domain.ErrInvalidRegistration
		}
		return nil, &domain.ValidationError{Cause: cause, Fields: fields}
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.userRepository.CreateUser(ctx, user); err != nil {
		if errors.Is(err, domain.ErrAccountExists) {
			conflicts, lookupErr := s.userRepository.RegistrationConflicts(ctx, email, username)
			if lookupErr != nil {
				return nil, lookupErr
			}
			var validation *domain.ValidationError
			if errors.As(err, &validation) {
				for field, message := range validation.Fields {
					conflicts[field] = message
				}
			}
			return nil, &domain.ValidationError{Cause: domain.ErrAccountExists, Fields: conflicts}
		}
		return nil, err
	}

	return &domain.RegisterResponse{
		UserID: user.ID,
	}, nil
}

func (s *Service) Login(ctx context.Context, identifier, password string) (*domain.LoginResponse, error) {
	identifier = strings.ToLower(strings.TrimSpace(identifier))

	if fields := loginErrors(identifier, password); len(fields) > 0 {
		return nil, &domain.ValidationError{Cause: domain.ErrInvalidLogin, Fields: fields}
	}
	if err := s.loginAttemptsRepository.CheckLocked(ctx, identifier); err != nil {
		return nil, err
	}

	user, err := s.userRepository.GetUserByIdentifier(ctx, identifier)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil, s.recordLoginFailure(ctx, identifier)
	}
	if err != nil {
		return nil, err
	}

	if !s.hasher.Compare(user.PasswordHash, password) {
		return nil, s.recordLoginFailure(ctx, identifier)
	}

	profile, err := s.userRepository.GetProfile(ctx, user.ID, user.ID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := &domain.Session{ID: uuid.New(), UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(s.refreshTokenTTL)}
	tokens, err := s.jwtService.Generate(user.ID.String(), user.Email, session.ID.String())
	if err != nil {
		return nil, err
	}

	refreshToken := s.newRefreshToken(user.ID, tokens.RefreshToken)
	refreshToken.SessionID = session.ID
	refreshToken.ExpiresAt = session.ExpiresAt

	if err := s.refreshTokenRepository.CreateSession(ctx, session, refreshToken, identifier); err != nil {
		return nil, err
	}

	return &domain.LoginResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    int64(time.Until(tokens.ExpiresAt).Seconds()),
		User: domain.UserResponse{
			ID:           user.ID,
			Email:        user.Email,
			Username:     user.Username,
			Privacy:      profile.Privacy,
			Description:  profile.Description,
			FriendsCount: profile.FriendsCount,
		},
	}, nil
}

func (s *Service) recordLoginFailure(ctx context.Context, identifier string) error {
	locked, err := s.loginAttemptsRepository.RecordFailure(ctx, identifier)
	if err != nil {
		return err
	}
	if locked {
		return domain.ErrLoginLocked
	}
	return domain.ErrBadCredentials
}

func (s *Service) Refresh(ctx context.Context, token string) (*domain.RefreshResponse, error) {
	if token == "" {
		return nil, domain.ErrInvalidRefreshToken
	}

	refreshToken, err := s.refreshTokenRepository.FindRefreshTokenByToken(ctx, token)
	if errors.Is(err, domain.ErrRefreshTokenNotFound) {
		return nil, domain.ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, err
	}

	if err := s.CheckSession(ctx, refreshToken.UserID, refreshToken.SessionID); err != nil {
		return nil, err
	}
	if refreshToken.IsRevoked || !refreshToken.ExpiresAt.After(time.Now().UTC()) {
		return nil, domain.ErrInvalidRefreshToken
	}

	user, err := s.userRepository.GetUserById(ctx, refreshToken.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil, domain.ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, err
	}

	tokens, err := s.jwtService.Generate(user.ID.String(), user.Email, refreshToken.SessionID.String())
	if err != nil {
		return nil, err
	}

	newRefreshToken := s.newRefreshToken(user.ID, tokens.RefreshToken)
	newRefreshToken.SessionID = refreshToken.SessionID
	newRefreshToken.ExpiresAt = refreshToken.ExpiresAt

	if err := s.refreshTokenRepository.RotateRefreshToken(ctx, token, newRefreshToken); err != nil {
		return nil, err
	}

	return &domain.RefreshResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    int64(time.Until(tokens.ExpiresAt).Seconds()),
	}, nil
}

func (s *Service) GetMe(ctx context.Context, userID uuid.UUID) (*domain.UserResponse, error) {
	user, err := s.userRepository.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}

	profile, err := s.userRepository.GetProfile(ctx, userID, userID)
	if err != nil {
		return nil, err
	}

	return &domain.UserResponse{
		ID:           user.ID,
		Email:        user.Email,
		Username:     user.Username,
		Privacy:      profile.Privacy,
		Description:  profile.Description,
		FriendsCount: profile.FriendsCount,
	}, nil
}

func (s *Service) CheckSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	return s.refreshTokenRepository.CheckSession(ctx, userID, sessionID)
}

func (s *Service) Logout(ctx context.Context, userID, sessionID uuid.UUID) error {
	return s.refreshTokenRepository.RevokeSession(ctx, userID, sessionID)
}

func (s *Service) newRefreshToken(userID uuid.UUID, token string) *domain.RefreshToken {
	now := time.Now().UTC()
	return &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		Token:     token,
		ExpiresAt: now.Add(s.refreshTokenTTL),
		CreatedAt: now,
		IsRevoked: false,
	}
}
