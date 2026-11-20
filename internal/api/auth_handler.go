package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/alina965/pLaNtS/user-service/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthService interface {
	Logout(ctx context.Context, userID, sessionID uuid.UUID) error
	Register(ctx context.Context, email, password, passwordConfirmation, username string) (*domain.RegisterResponse, error)
	Login(ctx context.Context, identifier, password string) (*domain.LoginResponse, error)
	Refresh(ctx context.Context, token string) (*domain.RefreshResponse, error)
	GetMe(ctx context.Context, userID uuid.UUID) (*domain.UserResponse, error)
}

type AuthHandler struct {
	service AuthService
}

func NewAuthHandler(service AuthService) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req domain.RegisterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest,
			"INVALID_JSON", "Некорректное тело запроса")
		return
	}

	response, err := h.service.Register(c.Request.Context(), req.Email, req.Password, req.PasswordConfirmation, req.Username)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req domain.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest,
			"INVALID_JSON", "Некорректное тело запроса")
		return
	}

	response, err := h.service.Login(c.Request.Context(), req.Identifier, req.Password)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req domain.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest,
			"INVALID_JSON", "Некорректное тело запроса")
		return
	}

	response, err := h.service.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) GetMe(c *gin.Context) {
	value, exists := c.Get("userID")
	userIDStr, ok := value.(string)

	if !exists || !ok {
		writeError(c, http.StatusUnauthorized,
			"UNAUTHORIZED", "Требуется авторизация")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(c, http.StatusUnauthorized,
			"UNAUTHORIZED", "Требуется авторизация")
		return
	}

	user, err := h.service.GetMe(c.Request.Context(), userID)
	if err != nil {
		writeServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	userID, userErr := uuid.Parse(c.GetString("userID"))
	sessionID, sessionErr := uuid.Parse(c.GetString("sessionID"))
	if userErr != nil || sessionErr != nil {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Требуется авторизация")
		return
	}
	if err := h.service.Logout(c.Request.Context(), userID, sessionID); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrLoginLocked):
		writeError(c, http.StatusTooManyRequests, "LOGIN_LOCKED", "Вход по этому идентификатору заблокирован на 1 час")
	case errors.Is(err, domain.ErrInvalidLogin):
		writeError(c, http.StatusBadRequest, "INVALID_LOGIN", "Некорректный email, имя пользователя или пароль")
	case errors.Is(err, domain.ErrSessionRevoked):
		writeError(c, http.StatusUnauthorized, "SESSION_REVOKED", "Сессия отозвана")
	case errors.Is(err, domain.ErrSessionExpired):
		writeError(c, http.StatusUnauthorized, "SESSION_EXPIRED", "Срок сессии истёк")
	case errors.Is(err, domain.ErrSessionUnavailable):
		writeError(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Проверка сессии временно недоступна")
	case errors.Is(err, domain.ErrInvalidRegistration):
		writeError(c, http.StatusBadRequest,
			"INVALID_REGISTRATION", "Некорректные данные регистрации")

	case errors.Is(err, domain.ErrAccountExists):
		writeError(c, http.StatusConflict,
			"ACCOUNT_EXISTS", "Email или имя пользователя уже заняты")

	case errors.Is(err, domain.ErrBadCredentials):
		writeError(c, http.StatusUnauthorized,
			"BAD_CREDENTIALS", "Неверный логин или пароль")

	case errors.Is(err, domain.ErrInvalidRefreshToken),
		errors.Is(err, domain.ErrRefreshTokenNotFound):
		writeError(c, http.StatusUnauthorized,
			"INVALID_REFRESH_TOKEN", "Недействительный refresh-токен")

	case errors.Is(err, domain.ErrUserNotFound):
		writeError(c, http.StatusUnauthorized,
			"UNAUTHORIZED", "Требуется авторизация")

	default:
		writeError(c, http.StatusInternalServerError,
			"INTERNAL_ERROR", "Внутренняя ошибка сервера")
	}
}
