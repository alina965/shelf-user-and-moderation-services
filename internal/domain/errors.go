package domain

import "errors"

var (
	ErrSessionRevoked       = errors.New("session revoked")
	ErrSessionExpired       = errors.New("session expired")
	ErrSessionUnavailable   = errors.New("session storage unavailable")
	ErrUserNotFound         = errors.New("user not found")
	ErrAccountExists        = errors.New("account already exists")
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
	ErrInvalidRefreshToken  = errors.New("invalid refresh token")
	ErrInvalidRegistration  = errors.New("invalid registration data")
	ErrBadCredentials       = errors.New("invalid identifier or password")
)
