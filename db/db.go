package db

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func Init(dbURL string) {
	// First, ensure the target database exists on the server
	if err := ensureDatabaseExists(dbURL); err != nil {
		log.Printf("[db] warning while ensuring database existence: %v", err)
	}

	var err error
	DB, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("[db] failed to open database: %v", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(time.Hour)

	if err = DB.Ping(); err != nil {
		log.Fatalf("[db] failed to ping database: %v", err)
	}

	migrate()
	log.Println("[db] database initialized successfully")
}

func ensureDatabaseExists(dbURL string) error {
	u, err := url.Parse(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database URL: %v", err)
	}

	targetDB := strings.TrimPrefix(u.Path, "/")
	if targetDB == "" || targetDB == "postgres" {
		return nil
	}

	// Create a connection string to the default 'postgres' database
	u.Path = "/postgres"
	adminConnStr := u.String()

	tempDB, err := sql.Open("postgres", adminConnStr)
	if err != nil {
		return err
	}
	defer tempDB.Close()

	// Check if database exists
	var exists bool
	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = '%s')", targetDB)
	err = tempDB.QueryRow(query).Scan(&exists)
	if err != nil {
		return err
	}

	if !exists {
		log.Printf("[db] Target database %q does not exist, creating it...", targetDB)
		// We can't use parameters for CREATE DATABASE name
		_, err = tempDB.Exec(fmt.Sprintf("CREATE DATABASE %s", targetDB))
		if err != nil {
			return fmt.Errorf("failed to create database: %v", err)
		}
		log.Printf("[db] Database %q created successfully.", targetDB)
	}

	return nil
}

func migrate() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id           BIGINT PRIMARY KEY,
			username     TEXT    DEFAULT '',
			full_name    TEXT    DEFAULT '',
			phone        TEXT    DEFAULT '',
			referred_by  BIGINT DEFAULT 0,
			referral_count INTEGER DEFAULT 0,
			referral_status INTEGER DEFAULT 0,
			is_admin     INTEGER DEFAULT 0,
			is_active    INTEGER DEFAULT 1,
			last_active  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS channels (
			id           SERIAL PRIMARY KEY,
			channel_id   TEXT    NOT NULL UNIQUE,
			channel_name TEXT    NOT NULL,
			channel_url  TEXT    NOT NULL DEFAULT '',
			is_active    INTEGER DEFAULT 1
		)`,

		`CREATE TABLE IF NOT EXISTS bot_settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,

		`CREATE TABLE IF NOT EXISTS admins (
			user_id  BIGINT PRIMARY KEY,
			added_by BIGINT NOT NULL DEFAULT 0,
			added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`SELECT 1`,
	}

	for _, q := range queries {
		if _, err := DB.Exec(q); err != nil {
			log.Fatalf("[db] migration failed: %v\nQuery: %s", err, q)
		}
	}
	
	_, _ = DB.Exec(`ALTER TABLE users ADD COLUMN referral_status INTEGER DEFAULT 0`)
	_, _ = DB.Exec(`ALTER TABLE users ADD COLUMN phone TEXT DEFAULT ''`)
	
	seedDefaultSettings()
	log.Println("[db] migrations applied")
}

func seedDefaultSettings() {
	defaults := map[string]string{
		"start_message": "Assalomu alaykum! Botga xush kelibsiz. 👋\n\nQuyidagi bo'limlardan foydalaning:",
		"start_video_file_id": "",
		"aksiya_text": "⚡️ Hozircha faol aksiyalar yo'q. Kuzatib boring!",
		"referral_ad_text": "🚀 Do'stingizni taklif qiling!\n\n🎁 Quyidagi tugma orqali botga qo'shiling va bonuslar yuting!\n\n👇 Pastdagi tugmani bosing:",
	}
	for key, val := range defaults {
		if _, err := DB.Exec(
			`INSERT INTO bot_settings (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING`, key, val,
		); err != nil {
			log.Printf("[db] failed to seed setting %q: %v", key, err)
		}
	}
	DB.Exec(`UPDATE bot_settings SET value = REPLACE(value, '\n', CHR(10)) WHERE value LIKE '%\n%'`)
}