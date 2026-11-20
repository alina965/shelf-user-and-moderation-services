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
