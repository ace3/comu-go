package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL   string
	RedisURL      string
	Port          string
	Env           string
	KAIAuthToken  string
	SyncSecret    string
	TelegramToken string
	OpenMeteoBase string
	Timezone      string
	AutoSync      bool
}

func Load() *Config {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("COMULINE_ENV")
	if env == "" {
		env = "development"
	}

	openMeteoBase := os.Getenv("OPEN_METEO_BASE")
	if openMeteoBase == "" {
		openMeteoBase = "https://api.open-meteo.com/v1/forecast"
	}

	tz := os.Getenv("TIMEZONE")
	if tz == "" {
		tz = "Asia/Jakarta"
	}

	autoSync := os.Getenv("AUTO_SYNC")

	return &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisURL:      os.Getenv("REDIS_URL"),
		Port:          port,
		Env:           env,
		KAIAuthToken:  os.Getenv("KAI_AUTH_TOKEN"),
		SyncSecret:    os.Getenv("SYNC_SECRET"),
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
		OpenMeteoBase: openMeteoBase,
		Timezone:      tz,
		AutoSync:      autoSync == "true" || autoSync == "1",
	}
}

// Validate checks that all required environment variables are set.
// Returns an error listing all missing variables at once.
func (c *Config) Validate() error {
	var missing []string

	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if c.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	if c.KAIAuthToken == "" {
		missing = append(missing, "KAI_AUTH_TOKEN")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// SetupLogging configures the default slog handler based on environment.
func SetupLogging(env string) {
	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	slog.SetDefault(slog.New(handler))
}
