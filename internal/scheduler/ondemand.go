package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/comu/api/internal/cache"
	"github.com/comu/api/internal/config"
	syncer "github.com/comu/api/internal/sync"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

var onDemandSyncGroup singleflight.Group

var onDemandState struct {
	mu          sync.Mutex
	lastTrigger time.Time
}

var onDemandSyncRunner = func(*config.Config, *gorm.DB, *cache.Cache) error { return nil }

func init() {
	onDemandSyncRunner = runOnDemandScheduleSync
}

func MaybeTriggerScheduleSync(cfg *config.Config, db *gorm.DB, c *cache.Cache, reason string) bool {
	if cfg == nil || !cfg.OnDemandSyncEnabled {
		return false
	}
	cooldown := time.Duration(cfg.OnDemandSyncMinIntervalMinutes) * time.Minute
	if cooldown <= 0 {
		cooldown = 30 * time.Minute
	}

	now := time.Now()
	onDemandState.mu.Lock()
	if !onDemandState.lastTrigger.IsZero() && now.Sub(onDemandState.lastTrigger) < cooldown {
		onDemandState.mu.Unlock()
		return false
	}
	onDemandState.lastTrigger = now
	onDemandState.mu.Unlock()

	go func() {
		_, _, _ = onDemandSyncGroup.Do("krl-schedule-on-demand", func() (any, error) {
			if err := onDemandSyncRunner(cfg, db, c); err != nil {
				slog.Error("on-demand schedule sync failed", "reason", reason, "error", err)
				return nil, err
			}
			slog.Info("on-demand schedule sync complete", "reason", reason)
			return nil, nil
		})
	}()
	return true
}

func runOnDemandScheduleSync(cfg *config.Config, db *gorm.DB, c *cache.Cache) error {
	ctx := context.Background()
	if db == nil {
		return nil
	}

	refreshToken(ctx, cfg)
	if err := syncer.SyncSchedules(cfg, db); err != nil {
		return err
	}
	if c != nil {
		c.InvalidateAll(ctx)
	}
	return nil
}

func resetOnDemandSyncState() {
	onDemandState.mu.Lock()
	onDemandState.lastTrigger = time.Time{}
	onDemandState.mu.Unlock()
}
