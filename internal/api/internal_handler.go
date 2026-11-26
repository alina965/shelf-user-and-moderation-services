package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/alina965/user-service/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ServiceAuthMiddleware(tokens map[string]string) gin.HandlerFunc {
	hashes := make(map[string][32]byte, len(tokens))
	for service, token := range tokens {
		hashes[service] = sha256.Sum256([]byte(token))
	}
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(hashes) == 0 {
			writeError(c, 503, "SERVICE_UNAVAILABLE", "Внутренний API не настроен")
			return
		}
		values := c.Request.Header.Values("Authorization")
		if len(values) != 1 {
			writeError(c, 401, "SERVICE_UNAUTHORIZED", "Требуется служебная авторизация")
			return
		}
		parts := strings.Fields(values[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(c, 401, "SERVICE_UNAUTHORIZED", "Требуется служебная авторизация")
			return
		}
		candidate := sha256.Sum256([]byte(parts[1]))
		matched := ""
		for service, hash := range hashes {
			if subtle.ConstantTimeCompare(candidate[:], hash[:]) == 1 {
				matched = service
			}
		}
		if matched == "" {
			writeError(c, 401, "SERVICE_UNAUTHORIZED", "Недействительный служебный токен")
			return
		}
		c.Set("callerService", matched)
		c.Next()
	}
}

func RequireServices(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, service := range allowed {
			if c.GetString("callerService") == service {
				c.Next()
				return
			}
		}
		writeError(c, 403, "FORBIDDEN", "У сервиса нет доступа к этому методу")
	}
}

type InternalService interface {
	Introspect(context.Context, string) (*domain.Identity, error)
	CheckPermission(context.Context, domain.PermissionCheckRequest) (*domain.PermissionCheckResponse, error)
	AccessChecks(context.Context, domain.AccessChecksRequest) (*domain.AccessChecksResponse, error)
	Previews(context.Context, []uuid.UUID) (*domain.PreviewsResponse, error)
	BlockedIDs(context.Context, string, uuid.UUID, int, string) (*domain.BlockedIDsResponse, error)
}

type InternalHandler struct{ service InternalService }

func NewInternalHandler(service InternalService) *InternalHandler {
	return &InternalHandler{service: service}
}

func internalJSON(c *gin.Context, value any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(c, 400, "INVALID_JSON", "Некорректное тело запроса")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(c, 400, "INVALID_JSON", "Ожидается один JSON-объект")
		return false
	}
	return true
}

func writeInternalError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInternalInput):
		writeError(c, 400, "INVALID_REQUEST", "Некорректные параметры внутреннего запроса")
	case errors.Is(err, domain.ErrInternalToken):
		writeError(c, 401, "UNAUTHORIZED", "Требуется действующий пользовательский access-токен")
	case errors.Is(err, domain.ErrInternalForbidden):
		writeError(c, 403, "FORBIDDEN", "Недостаточно прав")
	default:
		writeServiceError(c, err)
	}
}

func (h *InternalHandler) Introspect(c *gin.Context) {
	var request domain.IntrospectRequest
	if !internalJSON(c, &request) {
		return
	}
	response, err := h.service.Introspect(c.Request.Context(), request.AccessToken)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *InternalHandler) CheckPermission(c *gin.Context) {
	var request domain.PermissionCheckRequest
	if !internalJSON(c, &request) {
		return
	}
	response, err := h.service.CheckPermission(c.Request.Context(), request)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *InternalHandler) AccessChecks(c *gin.Context) {
	var request domain.AccessChecksRequest
	if !internalJSON(c, &request) {
		return
	}
	if c.GetString("callerService") != "collection" {
		for _, check := range request.Checks {
			if check.Section == "collection" {
				writeInternalError(c, domain.ErrInternalForbidden)
				return
			}
		}
	}
	response, err := h.service.AccessChecks(c.Request.Context(), request)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *InternalHandler) Previews(c *gin.Context) {
	var request domain.PreviewsRequest
	if !internalJSON(c, &request) {
		return
	}
	response, err := h.service.Previews(c.Request.Context(), request.UserIDs)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *InternalHandler) BlockedIDs(c *gin.Context) {
	owner, err := uuid.Parse(c.Param("id"))
	if err != nil || owner == uuid.Nil {
		writeInternalError(c, domain.ErrInternalInput)
		return
	}
	limit, err := pageLimit(c)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	values := c.Request.Header.Values("X-User-Authorization")
	if len(values) != 1 {
		writeInternalError(c, domain.ErrInternalToken)
		return
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeInternalError(c, domain.ErrInternalToken)
		return
	}
	response, err := h.service.BlockedIDs(c.Request.Context(), parts[1], owner, limit, c.Query("cursor"))
	if err != nil {
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}
