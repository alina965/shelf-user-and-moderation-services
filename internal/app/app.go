package app

import (
	"context"
	"log"
	"net/http"

	"github.com/alina965/user-service/internal/api"
	"github.com/alina965/user-service/internal/auth"
	"github.com/alina965/user-service/internal/config"
	"github.com/alina965/user-service/internal/friendship"
	"github.com/alina965/user-service/internal/interservice"
	"github.com/alina965/user-service/internal/jwt"
	"github.com/alina965/user-service/internal/repository"
	"github.com/alina965/user-service/internal/security"
	"github.com/gin-gonic/gin"

	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	db     *pgxpool.Pool
	server *http.Server
}

func New(config *config.Config) (*App, error) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return nil, err
	}

	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewRefreshTokenRepository(db)
	attemptsRepo := repository.NewLoginAttemptsRepository(db)
	hasher := security.NewBcryptHasher(config.BcryptCost)
	jwtService := jwt.NewJWTService([]byte(config.JwtSecret), config.AccessTokenTTL)
	userService := auth.NewAuthService(hasher, jwtService, tokenRepo, userRepo, attemptsRepo, config.RefreshTokenTTL)
	authHandler := api.NewAuthHandler(userService)
	friendshipHandler := api.NewFriendshipHandler(friendship.NewService(repository.NewFriendshipRepository(db)))
	internalHandler := api.NewInternalHandler(interservice.NewService(repository.NewInternalRepository(db), jwtService))
	healthHandler := api.NewHealthHandler(db)

	router := gin.Default()
	router.Use(api.RequestIDMiddleware())
	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)

	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.GET("/users/me", api.AuthMiddleware(jwtService, userService), authHandler.GetMe)
	router.GET("/users", authHandler.SearchUsers)
	router.GET("/users/:id", api.OptionalAuthMiddleware(jwtService, userService), authHandler.GetProfile)
	router.POST("/auth/logout", api.AuthMiddleware(jwtService, userService), authHandler.Logout)
	router.POST("/users/:id/friend-requests", api.AuthMiddleware(jwtService, userService), friendshipHandler.Send)
	router.GET("/friend-requests", api.AuthMiddleware(jwtService, userService), friendshipHandler.Requests)
	router.POST("/friend-requests/:id/accept", api.AuthMiddleware(jwtService, userService), friendshipHandler.Accept)
	router.DELETE("/friend-requests/:id", api.AuthMiddleware(jwtService, userService), friendshipHandler.Dismiss)
	router.DELETE("/users/me/friends/:user_id", api.AuthMiddleware(jwtService, userService), friendshipHandler.Remove)
	router.GET("/users/:id/friends", api.OptionalAuthMiddleware(jwtService, userService), friendshipHandler.Friends)
	router.PUT("/users/me/blocks/:user_id", api.AuthMiddleware(jwtService, userService), friendshipHandler.Block)
	router.DELETE("/users/me/blocks/:user_id", api.AuthMiddleware(jwtService, userService), friendshipHandler.Unblock)
	router.GET("/users/me/blocks", api.AuthMiddleware(jwtService, userService), friendshipHandler.Blocks)
	internal := router.Group("/internal", api.ServiceAuthMiddleware(config.InternalServiceTokens))
	internal.POST("/auth/introspect", internalHandler.Introspect)
	internal.POST("/auth/permission-check", api.RequireServices("moderation"), internalHandler.CheckPermission)
	internal.POST("/users/access-check", api.RequireServices("collection", "community"), internalHandler.AccessChecks)
	internal.POST("/users/previews", internalHandler.Previews)
	internal.GET("/users/:id/blocked-user-ids", api.RequireServices("community"), internalHandler.BlockedIDs)

	return &App{db: db, server: &http.Server{Addr: config.Addr, Handler: router}}, nil
}

func (a *App) Run() error {
	log.Println("Server started on " + a.server.Addr)
	return a.server.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	_ = a.server.Shutdown(ctx)
	a.db.Close()
	return nil
}
