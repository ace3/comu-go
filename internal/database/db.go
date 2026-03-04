package database

import (
	"log/slog"
	"os"
	"time"

	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/models"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	migrations "github.com/comuline/api/migrations"
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

	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
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

	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
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
