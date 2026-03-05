package main

import (
	"testing"

	"github.com/comu/api/internal/cache"
	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBootstrapTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Station{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestEnsureInitialStationData(t *testing.T) {
	t.Run("runs full sync when no stations exist", func(t *testing.T) {
		db := setupBootstrapTestDB(t)
		called := 0

		err := ensureInitialStationData(nil, db, nil, func(_ *config.Config, _ *gorm.DB, _ *cache.Cache) error {
			called++
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called != 1 {
			t.Fatalf("sync called %d times, expected 1", called)
		}
	})

	t.Run("does not run sync when stations already exist", func(t *testing.T) {
		db := setupBootstrapTestDB(t)
		if err := db.Create(&models.Station{UID: "manggarai", ID: "MRI", Name: "Manggarai", Type: "KRL"}).Error; err != nil {
			t.Fatalf("seed station: %v", err)
		}

		called := 0
		err := ensureInitialStationData(nil, db, nil, func(_ *config.Config, _ *gorm.DB, _ *cache.Cache) error {
			called++
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called != 0 {
			t.Fatalf("sync called %d times, expected 0", called)
		}
	})
}
