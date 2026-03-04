package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/sync"
	"gorm.io/gorm"
)

// Start launches a background goroutine that runs a full sync every day at
// 00:10 WIB (Asia/Jakarta). It stops cleanly when ctx is cancelled.
func Start(ctx context.Context, cfg *config.Config, db *gorm.DB) {
	go func() {
		for {
			next := nextRunTime()
			log.Printf("[scheduler] next auto-sync at %s", next.Format("2006-01-02 15:04:05 MST"))

			select {
			case <-time.After(time.Until(next)):
				run(cfg, db)
			case <-ctx.Done():
				log.Println("[scheduler] stopped")
				return
			}
		}
	}()
}

// RunNow executes a full sync immediately (used by the manual trigger endpoint).
func RunNow(cfg *config.Config, db *gorm.DB) error {
	log.Println("[scheduler] manual sync triggered")

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

	log.Println("[scheduler] manual sync complete")
	return nil
}

func run(cfg *config.Config, db *gorm.DB) {
	log.Println("[scheduler] starting auto-sync...")

	if err := sync.SyncStations(cfg, db); err != nil {
		log.Printf("[scheduler] station sync failed: %v", err)
		return
	}
	if err := sync.SyncSchedules(cfg, db); err != nil {
		log.Printf("[scheduler] schedule sync failed: %v", err)
		return
	}
	if err := sync.SyncMRTStations(db); err != nil {
		log.Printf("[scheduler] MRT station sync failed: %v", err)
		return
	}
	if err := sync.SyncMRTSchedules(db); err != nil {
		log.Printf("[scheduler] MRT schedule sync failed: %v", err)
		return
	}

	log.Println("[scheduler] auto-sync complete")
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
