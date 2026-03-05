package sync

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupScheduleSyncTestDB(t *testing.T) *gorm.DB {
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

func TestSyncSchedules_UsesParallelWorkers(t *testing.T) {
	db := setupScheduleSyncTestDB(t)

	for i := 0; i < 12; i++ {
		id := "ST" + string(rune('A'+i))
		if err := db.Create(&models.Station{UID: id, ID: id, Name: id, Type: "KRL"}).Error; err != nil {
			t.Fatalf("seed station %s: %v", id, err)
		}
	}

	original := syncStationSchedulesFunc
	defer func() { syncStationSchedulesFunc = original }()

	var active int32
	var maxActive int32
	var calls int32

	syncStationSchedulesFunc = func(_ *config.Config, _ *gorm.DB, _ string) error {
		cur := atomic.AddInt32(&active, 1)
		for {
			max := atomic.LoadInt32(&maxActive)
			if cur <= max {
				break
			}
			if atomic.CompareAndSwapInt32(&maxActive, max, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&calls, 1)
		atomic.AddInt32(&active, -1)
		return nil
	}

	if err := SyncSchedules(&config.Config{}, db); err != nil {
		t.Fatalf("SyncSchedules() error = %v", err)
	}

	if calls != 12 {
		t.Fatalf("calls = %d, expected 12", calls)
	}
	if maxActive < 2 {
		t.Fatalf("max concurrent workers = %d, expected > 1", maxActive)
	}
	if maxActive > scheduleSyncParallelism {
		t.Fatalf("max concurrent workers = %d, expected <= %d", maxActive, scheduleSyncParallelism)
	}
}

func TestSyncSchedules_OnlyKRLStationsAreSynced(t *testing.T) {
	db := setupScheduleSyncTestDB(t)

	seed := []models.Station{
		{UID: "mri", ID: "MRI", Name: "Manggarai", Type: "KRL"},
		{UID: "jakk", ID: "JAKK", Name: "Jakarta Kota", Type: "KRL"},
		{UID: "ftm", ID: "FTM", Name: "Fatmawati", Type: "MRT"},
		{UID: "cpr", ID: "CPR", Name: "Cipete Raya", Type: "MRT"},
	}
	for _, st := range seed {
		if err := db.Create(&st).Error; err != nil {
			t.Fatalf("seed station %s: %v", st.ID, err)
		}
	}

	original := syncStationSchedulesFunc
	defer func() { syncStationSchedulesFunc = original }()

	called := make(chan string, 10)
	syncStationSchedulesFunc = func(_ *config.Config, _ *gorm.DB, stationID string) error {
		called <- stationID
		return nil
	}

	if err := SyncSchedules(&config.Config{}, db); err != nil {
		t.Fatalf("SyncSchedules() error = %v", err)
	}
	close(called)

	got := make([]string, 0, 10)
	for id := range called {
		got = append(got, id)
	}
	slices.Sort(got)
	want := []string{"JAKK", "MRI"}
	if !slices.Equal(got, want) {
		t.Fatalf("synced station IDs = %v, want %v", got, want)
	}
}
