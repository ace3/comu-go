package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/comu/api/internal/cache"
	"github.com/comu/api/internal/config"
	"gorm.io/gorm"
)

func TestMaybeTriggerScheduleSync_Debounced(t *testing.T) {
	resetOnDemandSyncState()

	var calls int32
	original := onDemandSyncRunner
	onDemandSyncRunner = func(*config.Config, *gorm.DB, *cache.Cache) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}
	defer func() { onDemandSyncRunner = original }()

	cfg := &config.Config{
		OnDemandSyncEnabled:            true,
		OnDemandSyncMinIntervalMinutes: 30,
	}
	first := MaybeTriggerScheduleSync(cfg, nil, nil, "test")
	second := MaybeTriggerScheduleSync(cfg, nil, nil, "test")

	if !first {
		t.Fatalf("first trigger should run")
	}
	if second {
		t.Fatalf("second trigger should be debounced")
	}

	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}
