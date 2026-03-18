package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	ListenAddr   string
	DatabaseURL  string
	JWTSecret    string
	MicroMDMURL  string
	MicroMDMKey  string
	VPPTokenPath string
	WebhookPath  string
	WebSocketURL string
}

func Load() *Config {
	// Load .env file if it exists (won't override existing env vars)
	if err := godotenv.Load(); err != nil {
		log.Println("config: no .env file found, using environment variables")
	}

	return &Config{
		ListenAddr:   envOr("LISTEN_ADDR", ":8080"),
		DatabaseURL:  envOr("DATABASE_URL", "postgres://mdm:mdm@localhost:5432/mdm?sslmode=disable"),
		JWTSecret:    envOr("JWT_SECRET", "change-me-in-production"),
		MicroMDMURL:  envOr("MICROMDM_URL", ""),
		MicroMDMKey:  envOr("MICROMDM_API_KEY", ""),
		VPPTokenPath: envOr("VPP_TOKEN_PATH", ""),
		WebhookPath:  envOr("WEBHOOK_PATH", "/webhook"),
		WebSocketURL: envOr("WEBSOCKET_URL", ""),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
