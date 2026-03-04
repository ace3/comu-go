package database

import (
	"log"

	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Init opens a PostgreSQL connection and runs AutoMigrate.
func Init(cfg *config.Config) *gorm.DB {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(&models.Station{}, &models.Schedule{}); err != nil {
		log.Fatalf("failed to run database migrations: %v", err)
	}

	return db
}
