package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL     string
	RedisURL        string
	Port            string
	Env             string
	KAIAuthToken    string // initial value loaded from env; use Token() at runtime
	SyncSecret      string
	SkippedStations []string
	OnDemandSyncEnabled            bool
	OnDemandSyncMinIntervalMinutes int
	TelegramToken   string
	OpenMeteoBase   string
	Timezone        string
	AutoSync        bool
	AdminTelegramID int64 // Telegram user ID to notify on token events

	liveToken atomic.Pointer[string] // hot-reloadable token, set by RotateToken
}

// Token returns the current KAI auth token. It prefers the live (rotated) value
// over the initial value loaded from the environment.
func (c *Config) Token() string {
	if p := c.liveToken.Load(); p != nil {
		return *p
	}
	return c.KAIAuthToken
}

// RotateToken replaces the in-memory token without restarting the process.
// The change takes effect immediately for all subsequent sync calls.
// Note: this does not persist the new token to disk — update KAI_AUTH_TOKEN
// in your .env / secrets manager and restart to make it permanent.
func (c *Config) RotateToken(newToken string) {
	c.liveToken.Store(&newToken)
	slog.Info("KAI auth token rotated in memory")
}

func Load() *Config {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("COMU_ENV")
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
	skippedStations := parseCSVEnvWithDefault("SKIPPED_STATIONS", []string{"GGL", "CKP", "BANDARA", "PWK"})
	onDemandSyncEnabled := os.Getenv("ON_DEMAND_SYNC_ENABLED")
	if onDemandSyncEnabled == "" {
		onDemandSyncEnabled = "true"
	}
	onDemandSyncMinIntervalMinutes := 30
	if raw := os.Getenv("ON_DEMAND_SYNC_MIN_INTERVAL_MINUTES"); strings.TrimSpace(raw) != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			onDemandSyncMinIntervalMinutes = parsed
		}
	}

	var adminID int64
	if s := os.Getenv("ADMIN_TELEGRAM_ID"); s != "" {
		adminID, _ = strconv.ParseInt(s, 10, 64)
	}

	return &Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisURL:        os.Getenv("REDIS_URL"),
		Port:            port,
		Env:             env,
		KAIAuthToken:    os.Getenv("KAI_AUTH_TOKEN"),
		SyncSecret:      os.Getenv("SYNC_SECRET"),
		SkippedStations: skippedStations,
		OnDemandSyncEnabled:            onDemandSyncEnabled == "true" || onDemandSyncEnabled == "1",
		OnDemandSyncMinIntervalMinutes: onDemandSyncMinIntervalMinutes,
		TelegramToken:   os.Getenv("TELEGRAM_TOKEN"),
		OpenMeteoBase:   openMeteoBase,
		Timezone:        tz,
		AutoSync:        autoSync == "true" || autoSync == "1",
		AdminTelegramID: adminID,
	}
}

func parseCSVEnvWithDefault(key string, fallback []string) []string {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		return append([]string(nil), fallback...)
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
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
