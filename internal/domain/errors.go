package domain

import "errors"

type ValidationError struct {
	Cause  error
	Fields map[string]string
}

func (e *ValidationError) Error() string { return e.Cause.Error() }

func (e *ValidationError) Unwrap() error { return e.Cause }

const EmailTakenMessage = "Учётная запись с таким адресом электронной почты уже существует"
const UsernameTakenMessage = "Это имя пользователя уже занято"

var (
	ErrAccessCheckUnavailable = errors.New("access check unavailable")
	ErrProfileNotFound        = errors.New("profile not found")
	ErrQueryTooShort          = errors.New("search query too short")
	ErrQueryRequired          = errors.New("search query required")
	ErrInvalidLimit           = errors.New("invalid page limit")
	ErrInvalidCursor          = errors.New("invalid search cursor")
	ErrSessionRevoked         = errors.New("session revoked")
	ErrSessionExpired         = errors.New("session expired")
	ErrSessionUnavailable     = errors.New("session storage unavailable")
	ErrUserNotFound           = errors.New("user not found")
	ErrAccountExists          = errors.New("account already exists")
	ErrRefreshTokenNotFound   = errors.New("refresh token not found")
	ErrInvalidRefreshToken    = errors.New("invalid refresh token")
	ErrInvalidRegistration    = errors.New("invalid registration data")
	ErrBadCredentials         = errors.New("invalid identifier or password")
	ErrProfilePrivacyNotFound = errors.New("profile privacy not found")
	LoginAttemptsNotFound     = errors.New("login attempts not found")
	ErrLoginLocked            = errors.New("login locked")
	ErrInvalidLogin           = errors.New("invalid login data")
)
