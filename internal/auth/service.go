package auth

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alina965/pLaNtS/user-service/internal/domain"

	"github.com/google/uuid"
)

type Service struct {
	hasher                 PasswordHasher
	jwtService             TokenService
	userRepository         UserRepository
	refreshTokenRepository RefreshTokenRepository
	refreshTokenTTL        time.Duration
}

func NewAuthService(hasher PasswordHasher, jwtService TokenService, refreshTokenRepository RefreshTokenRepository, userRepository UserRepository, refreshTokenTTL time.Duration) *Service {
	return &Service{
		hasher:                 hasher,
		jwtService:             jwtService,
		refreshTokenRepository: refreshTokenRepository,
		userRepository:         userRepository,
		refreshTokenTTL:        refreshTokenTTL,
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
	CreateSession(ctx context.Context, session *domain.Session, token *domain.RefreshToken) error
	CheckSession(ctx context.Context, userID, sessionID uuid.UUID) error
	RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error
	FindRefreshTokenByToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	RotateRefreshToken(ctx context.Context, oldToken string, newToken *domain.RefreshToken) error
}

type UserRepository interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserById(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetUserByIdentifier(ctx context.Context, identifier string) (*domain.User, error)
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.]{1,32}$`)

func (s *Service) Register(ctx context.Context, email, password, passwordConfirmation, username string) (*domain.RegisterResponse, error) {
	email = strings.TrimSpace(email)
	username = strings.TrimSpace(username)

	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || utf8.RuneCountInString(email) > 254 {
		return nil, domain.ErrInvalidRegistration
	}

	if !usernamePattern.MatchString(username) ||
		strings.HasPrefix(username, ".") ||
		strings.HasSuffix(username, ".") ||
		strings.Contains(username, "..") {
		return nil, domain.ErrInvalidRegistration
	}

	passwordLength := utf8.RuneCountInString(password)
	if passwordLength < 8 || passwordLength > 32 || password != passwordConfirmation {
		return nil, domain.ErrInvalidRegistration
	}

	if len(password) > 72 {
		return nil, domain.ErrInvalidRegistration
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
		return nil, err
	}

	return &domain.RegisterResponse{
		UserID: user.ID,
	}, nil
}

func (s *Service) Login(ctx context.Context, identifier, password string) (*domain.LoginResponse, error) {
	identifier = strings.TrimSpace(identifier)

	if identifier == "" || password == "" {
		return nil, domain.ErrBadCredentials
	}

	user, err := s.userRepository.GetUserByIdentifier(ctx, identifier)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil, domain.ErrBadCredentials
	}
	if err != nil {
		return nil, err
	}

	if !s.hasher.Compare(user.PasswordHash, password) {
		return nil, domain.ErrBadCredentials
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

	if err := s.refreshTokenRepository.CreateSession(ctx, session, refreshToken); err != nil {
		return nil, err
	}

	return &domain.LoginResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    int64(time.Until(tokens.ExpiresAt).Seconds()),
		User: domain.UserResponse{
			ID:       user.ID,
			Email:    user.Email,
			Username: user.Username,
		},
	}, nil
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

	return &domain.UserResponse{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
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
