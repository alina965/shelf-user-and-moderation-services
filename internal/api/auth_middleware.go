package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TokenParser interface {
	Parse(accessToken string) (userID string, sessionID string, err error)
}

type SessionChecker interface {
	CheckSession(context.Context, uuid.UUID, uuid.UUID) error
}

func ensureRequestID(c *gin.Context) string {
	requestID := c.GetString("requestID")
	if requestID == "" {
		requestID = "req_" + uuid.NewString()
		c.Set("requestID", requestID)
	}
	c.Header("X-Request-ID", requestID)
	return requestID
}

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ensureRequestID(c)
		c.Next()
	}
}

func OptionalAuthMiddleware(parser TokenParser, sessions SessionChecker) gin.HandlerFunc {
	required := AuthMiddleware(parser, sessions)
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if _, present := c.Request.Header["Authorization"]; !present {
			c.Next()
			return
		}
		required(c)
	}
}

func AuthMiddleware(parser TokenParser, sessions SessionChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))

		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(c, http.StatusUnauthorized,
				"UNAUTHORIZED", "Требуется авторизация")
			return
		}

		userID, sessionID, err := parser.Parse(parts[1])
		if err != nil {
			writeError(c, http.StatusUnauthorized,
				"UNAUTHORIZED", "Недействительный или просроченный токен")
			return
		}

		uid, userErr := uuid.Parse(userID)
		sid, sessionErr := uuid.Parse(sessionID)
		if userErr != nil || sessionErr != nil {
			writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Недействительный токен")
			return
		}
		if err := sessions.CheckSession(c.Request.Context(), uid, sid); err != nil {
			writeServiceError(c, err)
			return
		}
		c.Set("userID", userID)
		c.Set("sessionID", sessionID)
		c.Next()
	}
}
