package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/alina965/user-service/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FriendshipService interface {
	Send(context.Context, uuid.UUID, uuid.UUID) (*domain.FriendRequest, error)
	Resolve(context.Context, uuid.UUID, uuid.UUID, bool) error
	Remove(context.Context, uuid.UUID, uuid.UUID) error
	Requests(context.Context, uuid.UUID, string, int, string) (*domain.FriendRequestsResponse, error)
	Friends(context.Context, uuid.UUID, uuid.UUID, int, string) (*domain.UserSearchResponse, error)
	SetBlock(context.Context, uuid.UUID, uuid.UUID, bool) error
	Blocks(context.Context, uuid.UUID, int, string) (*domain.UserSearchResponse, error)
}

type FriendshipHandler struct {
	service FriendshipService
}

func NewFriendshipHandler(service FriendshipService) *FriendshipHandler {
	return &FriendshipHandler{service: service}
}

func pageLimit(c *gin.Context) (int, error) {
	values, exists := c.Request.URL.Query()["limit"]
	if !exists {
		return 20, nil
	}
	limit, err := strconv.Atoi(values[0])
	if err != nil || limit < 1 || limit > 20 {
		return 0, domain.ErrInvalidLimit
	}
	return limit, nil
}

func actorID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.GetString("userID"))
	if err != nil || id == uuid.Nil {
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Требуется авторизация")
		return uuid.Nil, false
	}
	return id, true
}

func (h *FriendshipHandler) Send(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	target, err := uuid.Parse(c.Param("id"))
	if err != nil || target == uuid.Nil {
		writeServiceError(c, domain.ErrProfileNotFound)
		return
	}
	request, err := h.service.Send(c.Request.Context(), actor, target)
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.JSON(http.StatusCreated, request)
}

func (h *FriendshipHandler) Requests(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := actorID(c)
	if !ok {
		return
	}
	limit, err := pageLimit(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response, err := h.service.Requests(c.Request.Context(), actor, c.Query("direction"), limit, c.Query("cursor"))
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *FriendshipHandler) Accept(c *gin.Context) { h.resolve(c, true) }

func (h *FriendshipHandler) Dismiss(c *gin.Context) { h.resolve(c, false) }

func (h *FriendshipHandler) resolve(c *gin.Context, accept bool) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		writeFriendshipError(c, domain.ErrRequestNotFound)
		return
	}
	if err := h.service.Resolve(c.Request.Context(), actor, id, accept); err != nil {
		writeFriendshipError(c, err)
		return
	}
	if accept {
		c.JSON(http.StatusOK, gin.H{"status": "accepted"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *FriendshipHandler) Remove(c *gin.Context) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("user_id"))
	if err != nil || id == uuid.Nil {
		writeServiceError(c, domain.ErrProfileNotFound)
		return
	}
	if err := h.service.Remove(c.Request.Context(), actor, id); err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *FriendshipHandler) Friends(c *gin.Context) {
	viewer := uuid.Nil
	if c.GetString("userID") != "" {
		var ok bool
		viewer, ok = actorID(c)
		if !ok {
			return
		}
	}
	owner, err := uuid.Parse(c.Param("id"))
	if c.Param("id") == "me" {
		if viewer == uuid.Nil {
			writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Требуется авторизация")
			return
		}
		owner, err = viewer, nil
	}
	if err != nil || owner == uuid.Nil {
		writeServiceError(c, domain.ErrProfileNotFound)
		return
	}
	limit, err := pageLimit(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response, err := h.service.Friends(c.Request.Context(), owner, viewer, limit, c.Query("cursor"))
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *FriendshipHandler) Block(c *gin.Context) { h.setBlock(c, true) }

func (h *FriendshipHandler) Unblock(c *gin.Context) { h.setBlock(c, false) }

func (h *FriendshipHandler) setBlock(c *gin.Context, enabled bool) {
	actor, ok := actorID(c)
	if !ok {
		return
	}
	target, err := uuid.Parse(c.Param("user_id"))
	if err != nil || target == uuid.Nil {
		writeServiceError(c, domain.ErrProfileNotFound)
		return
	}
	if err := h.service.SetBlock(c.Request.Context(), actor, target, enabled); err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *FriendshipHandler) Blocks(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := actorID(c)
	if !ok {
		return
	}
	limit, err := pageLimit(c)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response, err := h.service.Blocks(c.Request.Context(), actor, limit, c.Query("cursor"))
	if err != nil {
		writeFriendshipError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func writeFriendshipError(c *gin.Context, err error) {
	var conflict *domain.RequestConflict
	if errors.As(err, &conflict) {
		code, message := "REQUEST_ALREADY_EXISTS", "Заявка уже отправлена"
		if conflict.Incoming {
			code, message = "INCOMING_REQUEST_EXISTS", "У вас уже есть входящая заявка от этого пользователя"
		}
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"request_id": ensureRequestID(c), "error": gin.H{
			"code": code, "message": message, "details": gin.H{"friend_request_id": conflict.ID},
		}})
		return
	}
	switch {
	case errors.Is(err, domain.ErrSelfFriendship):
		writeError(c, http.StatusBadRequest, "SELF_ACTION", "Нельзя выполнить действие с самим собой")
	case errors.Is(err, domain.ErrInvalidDirection):
		writeError(c, http.StatusBadRequest, "INVALID_DIRECTION", "Укажите direction=incoming или outgoing")
	case errors.Is(err, domain.ErrInteractionBlocked):
		writeError(c, http.StatusForbidden, "INTERACTION_BLOCKED", "Взаимодействие недоступно")
	case errors.Is(err, domain.ErrRequestRecipient):
		writeError(c, http.StatusForbidden, "FORBIDDEN", "Принять заявку может только получатель")
	case errors.Is(err, domain.ErrRequestNotFound):
		writeError(c, http.StatusNotFound, "FRIEND_REQUEST_NOT_FOUND", "Заявка не найдена")
	case errors.Is(err, domain.ErrFriendsHidden):
		writeError(c, http.StatusForbidden, "FORBIDDEN", "Список друзей недоступен")
	case errors.Is(err, domain.ErrAlreadyFriends):
		writeError(c, http.StatusConflict, "ALREADY_FRIENDS", "Вы уже друзья")
	case errors.Is(err, domain.ErrRequestState):
		writeError(c, http.StatusConflict, "FRIEND_REQUEST_NOT_PENDING", "Заявка больше не ожидает ответа")
	default:
		writeServiceError(c, err)
	}
}
