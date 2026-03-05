package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/comu/api/internal/cache"
	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/sync"
	"github.com/comu/api/internal/tokenrefresh"
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
				run(ctx, cfg, db, c)
			case <-ctx.Done():
				slog.Info("scheduler stopped")
				return
			}
		}
	}()
}

// RunNow executes a full sync immediately (used by the manual trigger endpoint).
func RunNow(cfg *config.Config, db *gorm.DB, c *cache.Cache) error {
	ctx := context.Background()
	slog.Info("manual sync triggered")

	refreshToken(ctx, cfg)

	if err := sync.SyncStations(cfg, db); err != nil {
		return err
	}
	if err := sync.SyncSchedules(cfg, db); err != nil {
		return err
	}
	if err := sync.SyncMRTStations(db); err != nil {
		return err
	}
	if err := sync.SyncMRTSchedules(db); err != nil {
		return err
	}

	c.InvalidateAll(ctx)
	slog.Info("manual sync complete")
	return nil
}

func run(ctx context.Context, cfg *config.Config, db *gorm.DB, c *cache.Cache) {
	slog.Info("starting auto-sync")

	refreshToken(ctx, cfg)

	if err := sync.SyncStations(cfg, db); err != nil {
		slog.Error("station sync failed", "error", err)
		if errors.Is(err, sync.ErrTokenExpired) {
			slog.Warn("skipping schedule sync due to expired token")
		}
		return
	}
	if err := sync.SyncSchedules(cfg, db); err != nil {
		slog.Error("schedule sync failed", "error", err)
		return
	}
	if err := sync.SyncMRTStations(db); err != nil {
		slog.Error("MRT station sync failed", "error", err)
		return
	}
	if err := sync.SyncMRTSchedules(db); err != nil {
		slog.Error("MRT schedule sync failed", "error", err)
		return
	}

	c.InvalidateAll(ctx)
	slog.Info("auto-sync complete")
}

// refreshToken checks token expiry and attempts to auto-rotate from the KCI page.
// All failures are logged as warnings — they never abort the sync.
func refreshToken(ctx context.Context, cfg *config.Config) {
	// 1. Warn admin if current token expires within 4 days.
	tokenrefresh.CheckExpiry(ctx, cfg.Token(), cfg.TelegramToken, cfg.AdminTelegramID, 4*24*time.Hour)

	// 2. Try to fetch a fresher token from the KCI page.
	rotated, err := tokenrefresh.TryRefresh(
		ctx,
		cfg.Token(),
		cfg.RotateToken,
		cfg.TelegramToken,
		cfg.AdminTelegramID,
	)
	if err != nil {
		slog.Warn("token auto-refresh failed", "error", err)
		return
	}
	if rotated {
		slog.Info("token auto-refreshed from KCI page")
	}
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
