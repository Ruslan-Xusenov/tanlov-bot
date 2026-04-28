package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	BotToken     string
	SuperAdminID int64
	DatabaseURL  string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("[config] .env file not found, reading from environment")
	}

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("[config] BOT_TOKEN is required")
	}

	adminIDStr := os.Getenv("SUPER_ADMIN_ID")
	adminID, err := strconv.ParseInt(adminIDStr, 10, 64)
	if err != nil || adminID == 0 {
		log.Fatal("[config] SUPER_ADMIN_ID must be a valid Telegram user ID (non-zero integer)")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/tanlov?sslmode=disable"
	}

	return &Config{
		BotToken:     token,
		SuperAdminID: adminID,
		DatabaseURL:  dbURL,
	}
}