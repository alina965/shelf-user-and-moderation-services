package app

import (
	"context"
	"log"
	"net/http"

	"github.com/alina965/pLaNtS/user-service/internal/api"
	"github.com/alina965/pLaNtS/user-service/internal/auth"
	"github.com/alina965/pLaNtS/user-service/internal/config"
	"github.com/alina965/pLaNtS/user-service/internal/jwt"
	"github.com/alina965/pLaNtS/user-service/internal/repository"
	"github.com/alina965/pLaNtS/user-service/internal/security"
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
	hasher := security.NewBcryptHasher(config.BcryptCost)
	jwtService := jwt.NewJWTService([]byte(config.JwtSecret), config.AccessTokenTTL)
	userService := auth.NewAuthService(hasher, jwtService, tokenRepo, userRepo, config.RefreshTokenTTL)
	authHandler := api.NewAuthHandler(userService)

	router := gin.Default()

	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.GET("/users/me", api.AuthMiddleware(jwtService, userService), authHandler.GetMe)
	router.POST("/auth/logout", api.AuthMiddleware(jwtService, userService), authHandler.Logout)

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
