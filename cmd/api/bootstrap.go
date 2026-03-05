package main

import (
	"fmt"
	"log/slog"

	"github.com/comu/api/internal/cache"
	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/models"
	"gorm.io/gorm"
)

type fullSyncFunc func(cfg *config.Config, db *gorm.DB, c *cache.Cache) error

// ensureInitialStationData triggers a full sync when the station table is empty.
func ensureInitialStationData(cfg *config.Config, db *gorm.DB, c *cache.Cache, runFullSync fullSyncFunc) error {
	var total int64
	if err := db.Model(&models.Station{}).Count(&total).Error; err != nil {
		return fmt.Errorf("counting stations: %w", err)
	}

	if total > 0 {
		return nil
	}

	slog.Warn("no stations found in database; running full sync")
	if err := runFullSync(cfg, db, c); err != nil {
		return fmt.Errorf("running initial full sync: %w", err)
	}

	return nil
}
