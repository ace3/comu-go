package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/comuline/api/internal/cache"
	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/sync"
	"gorm.io/gorm"
)

// Start launches a background goroutine that runs a full sync every day at
// 00:10 WIB (Asia/Jakarta). It stops cleanly when ctx is cancelled.
func Start(ctx context.Context, cfg *config.Config, db *gorm.DB, c *cache.Cache) {
	go func() {
		for {
			next := nextRunTime()
			slog.Info("next auto-sync scheduled", "time", next.Format("2006-01-02 15:04:05 MST"))

			select {
			case <-time.After(time.Until(next)):
				run(cfg, db, c)
			case <-ctx.Done():
				slog.Info("scheduler stopped")
				return
			}
		}
	}()
}

// RunNow executes a full sync immediately (used by the manual trigger endpoint).
func RunNow(cfg *config.Config, db *gorm.DB, c *cache.Cache) error {
	slog.Info("manual sync triggered")

	if err := sync.SyncStations(cfg, db); err != nil {
		return err
	}
	if err := sync.SyncSchedules(cfg, db); err != nil {
		return err
	}

	c.InvalidateAll(context.Background())
	slog.Info("manual sync complete")
	return nil
}

func run(cfg *config.Config, db *gorm.DB, c *cache.Cache) {
	slog.Info("starting auto-sync")

	if err := sync.SyncStations(cfg, db); err != nil {
		slog.Error("station sync failed", "error", err)
		if errors.Is(err, sync.ErrTokenExpired) {
			slog.Warn("skipping schedule sync due to expired token")
			return
		}
		return
	}
	if err := sync.SyncSchedules(cfg, db); err != nil {
		slog.Error("schedule sync failed", "error", err)
		return
	}

	c.InvalidateAll(context.Background())
	slog.Info("auto-sync complete")
}

// nextRunTime returns the next 00:10 WIB timestamp after now.
func nextRunTime() time.Time {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 10, 0, 0, loc)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}
