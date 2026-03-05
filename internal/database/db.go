package database

import (
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/models"
	migrations "github.com/comu/api/migrations"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Init opens a PostgreSQL connection, configures the connection pool, and runs migrations.
func Init(cfg *config.Config) *gorm.DB {
	logMode := logger.Info
	if cfg.Env == "production" {
		logMode = logger.Warn
	}

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logMode),
	})
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("failed to get underlying sql.DB", "error", err)
		os.Exit(1)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	if cfg.Env == "production" {
		RunMigrations(cfg.DatabaseURL)
	} else {
		if err := db.AutoMigrate(&models.Station{}, &models.Schedule{}); err != nil {
			slog.Error("failed to run database migrations", "error", err)
			os.Exit(1)
		}
	}

	return db
}

// RunMigrations runs SQL migrations from the embedded migrations directory.
func RunMigrations(databaseURL string) {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		slog.Error("failed to create migration source", "error", err)
		os.Exit(1)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, normalizeDatabaseURLForMigrate(databaseURL))
	if err != nil {
		slog.Error("failed to create migrate instance", "error", err)
		os.Exit(1)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	slog.Info("database migrations applied successfully")
}

// RunMigrationsDown rolls back all SQL migrations.
func RunMigrationsDown(databaseURL string) {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		slog.Error("failed to create migration source", "error", err)
		os.Exit(1)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, normalizeDatabaseURLForMigrate(databaseURL))
	if err != nil {
		slog.Error("failed to create migrate instance", "error", err)
		os.Exit(1)
	}

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		slog.Error("failed to roll back migrations", "error", err)
		os.Exit(1)
	}

	slog.Info("database migrations rolled back successfully")
}

// normalizeDatabaseURLForMigrate ensures local PostgreSQL URLs work with lib/pq defaults.
// lib/pq defaults sslmode=require when omitted, which fails against local dev servers
// that commonly run without TLS.
func normalizeDatabaseURLForMigrate(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	query := parsed.Query()
	if query.Get("sslmode") != "" {
		return raw
	}

	host := parsed.Hostname()
	if !isLocalDBHost(host) {
		return raw
	}

	query.Set("sslmode", "disable")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func isLocalDBHost(host string) bool {
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
