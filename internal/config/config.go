package config

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	defaultAddress         = ":8080"
	defaultAccessTokenTTL  = time.Minute * 15
	defaultRefreshTokenTTL = 7 * 24 * time.Hour
)

type Config struct {
	Addr                  string
	DatabaseURL           string
	JwtSecret             string
	AccessTokenTTL        time.Duration
	RefreshTokenTTL       time.Duration
	InternalServiceTokens map[string]string
}

func New(path string) (*Config, error) {
	_ = godotenv.Load(path)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddress
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, errors.New("JWT_SECRET environment variable not set")
	}
	accessTokenTTL, err := time.ParseDuration(os.Getenv("ACCESS_TOKEN_TTL"))
	if err != nil {
		accessTokenTTL = defaultAccessTokenTTL
	}
	refreshTokenTTL, err := time.ParseDuration(os.Getenv("REFRESH_TOKEN_TTL"))
	if err != nil {
		refreshTokenTTL = defaultRefreshTokenTTL
	}
	internalTokens := make(map[string]string)
	if value := os.Getenv("INTERNAL_SERVICE_TOKENS"); value != "" {
		if err := json.Unmarshal([]byte(value), &internalTokens); err != nil || internalTokens == nil {
			return nil, errors.New("INTERNAL_SERVICE_TOKENS must be a JSON object")
		}
		seen := make(map[string]bool)
		for service, token := range internalTokens {
			if service != "collection" && service != "community" && service != "catalog" && service != "moderation" {
				return nil, errors.New("unknown service in INTERNAL_SERVICE_TOKENS")
			}
			if len(token) < 32 || strings.ContainsAny(token, " \t\r\n") || token == jwtSecret || seen[token] {
				return nil, errors.New("internal tokens must be distinct, at least 32 bytes and different from JWT_SECRET")
			}
			seen[token] = true
		}
	}

	return &Config{
		Addr:                  addr,
		DatabaseURL:           databaseURL,
		JwtSecret:             jwtSecret,
		AccessTokenTTL:        accessTokenTTL,
		RefreshTokenTTL:       refreshTokenTTL,
		InternalServiceTokens: internalTokens,
	}, nil
}
