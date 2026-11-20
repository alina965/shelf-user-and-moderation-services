package jwt

import (
	"fmt"
	"time"

	"github.com/alina965/pLaNtS/user-service/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Service struct {
	secret         []byte
	accessTokenTTl time.Duration
}

type AccessClaims struct {
	SessionID string `json:"sid"`
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	jwt.RegisteredClaims
}

func NewJWTService(secret []byte, accessTokenTTL time.Duration) *Service {
	return &Service{secret, accessTokenTTL}
}

func (s *Service) Generate(userID string, email string, sessionID string) (*domain.TokenPair, error) {
	now := time.Now()

	claims := AccessClaims{
		SessionID: sessionID,
		UserID:    userID,
		Email:     email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return nil, err
	}

	return &domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: uuid.NewString(),
		ExpiresAt:    claims.ExpiresAt.Time,
	}, nil
}

func (s *Service) Parse(accessToken string) (string, string, error) {
	token, err := jwt.ParseWithClaims(accessToken, &AccessClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return "", "", err
	}
	if claims, ok := token.Claims.(*AccessClaims); ok && token.Valid {
		if _, err := uuid.Parse(claims.UserID); err != nil {
			return "", "", fmt.Errorf("invalid user ID")
		}
		if _, err := uuid.Parse(claims.SessionID); err != nil {
			return "", "", fmt.Errorf("invalid session ID")
		}
		return claims.UserID, claims.SessionID, nil
	}
	return "", "", fmt.Errorf("invalid access token")
}
