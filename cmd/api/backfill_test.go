package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/comu/api/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBackfillTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Station{}, &models.Schedule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestBackfillFromDataIfEmpty(t *testing.T) {
	dir := t.TempDir()

	stationsJSON := `[
  {
    "uid": "manggarai",
    "id": "MRI",
    "name": "Manggarai",
    "type": "KRL",
    "metadata": "{\"daop\":1,\"fg_enable\":1}",
    "created_at": "0001-01-01 00:00:00+00",
    "updated_at": "2026-03-05 05:10:13.406178+00"
  }
]`
	schedulesJSON := `[
  {
    "id": "MRI-2200",
    "train_id": "2200",
    "line": "Commuter Line Bogor",
    "route": "BOGOR-JAKARTAKOTA",
    "origin_id": "BOO",
    "destination_id": "JAKK",
    "station_id": "MRI",
    "departs_at": "2026-03-03 22:07:30+00",
    "arrives_at": "2026-03-03 22:16:00+00",
    "metadata": "{\"dest\":\"Jakartakota\",\"ka_name\":\"Commuter Line Bogor\"}",
    "created_at": "2026-03-04 14:51:42.497487+00",
    "updated_at": "2026-03-04 14:54:45.506784+00"
  }
]`

	if err := os.WriteFile(filepath.Join(dir, "stations.json"), []byte(stationsJSON), 0o644); err != nil {
		t.Fatalf("write stations.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schedules.json"), []byte(schedulesJSON), 0o644); err != nil {
		t.Fatalf("write schedules.json: %v", err)
	}

	t.Run("backfills both tables when empty", func(t *testing.T) {
		db := setupBackfillTestDB(t)
		if err := backfillFromDataIfEmpty(db, dir); err != nil {
			t.Fatalf("backfillFromDataIfEmpty() error = %v", err)
		}

		var stationCount int64
		var scheduleCount int64
		if err := db.Model(&models.Station{}).Count(&stationCount).Error; err != nil {
			t.Fatalf("count stations: %v", err)
		}
		if err := db.Model(&models.Schedule{}).Count(&scheduleCount).Error; err != nil {
			t.Fatalf("count schedules: %v", err)
		}
		if stationCount != 1 {
			t.Fatalf("station count = %d, expected 1", stationCount)
		}
		if scheduleCount != 1 {
			t.Fatalf("schedule count = %d, expected 1", scheduleCount)
		}
	})

	t.Run("skips backfill when tables already contain data", func(t *testing.T) {
		db := setupBackfillTestDB(t)
		if err := db.Create(&models.Station{UID: "mri", ID: "MRI", Name: "Manggarai", Type: "KRL"}).Error; err != nil {
			t.Fatalf("seed station: %v", err)
		}
		if err := db.Create(&models.Schedule{ID: "x", TrainID: "x", Line: "x", Route: "x", StationID: "MRI"}).Error; err != nil {
			t.Fatalf("seed schedule: %v", err)
		}

		if err := backfillFromDataIfEmpty(db, dir); err != nil {
			t.Fatalf("backfillFromDataIfEmpty() error = %v", err)
		}

		var stationCount int64
		var scheduleCount int64
		_ = db.Model(&models.Station{}).Count(&stationCount)
		_ = db.Model(&models.Schedule{}).Count(&scheduleCount)
		if stationCount != 1 || scheduleCount != 1 {
			t.Fatalf("counts changed unexpectedly, stations=%d schedules=%d", stationCount, scheduleCount)
		}
	})

	t.Run("does not fail when backfill files are missing", func(t *testing.T) {
		db := setupBackfillTestDB(t)
		missingDir := filepath.Join(dir, "missing")
		if err := backfillFromDataIfEmpty(db, missingDir); err != nil {
			t.Fatalf("backfillFromDataIfEmpty() should ignore missing files, got error = %v", err)
		}

		var stationCount int64
		var scheduleCount int64
		_ = db.Model(&models.Station{}).Count(&stationCount)
		_ = db.Model(&models.Schedule{}).Count(&scheduleCount)
		if stationCount != 0 || scheduleCount != 0 {
			t.Fatalf("expected empty tables, got stations=%d schedules=%d", stationCount, scheduleCount)
		}
	})
}

func TestBackfillFromDataForce(t *testing.T) {
	dir := t.TempDir()

	stationsJSON := `[
  {
    "uid": "manggarai",
    "id": "MRI",
    "name": "Manggarai",
    "type": "KRL",
    "metadata": "{\"daop\":1,\"fg_enable\":1}"
  }
]`
	schedulesJSON := `[
  {
    "id": "MRI-2200",
    "train_id": "2200",
    "line": "Commuter Line Bogor",
    "route": "BOGOR-JAKARTAKOTA",
    "origin_id": "BOO",
    "destination_id": "JAKK",
    "station_id": "MRI",
    "departs_at": "2026-03-03 22:07:30+00",
    "arrives_at": "2026-03-03 22:16:00+00",
    "metadata": "{\"dest\":\"Jakartakota\",\"ka_name\":\"Commuter Line Bogor\"}"
  }
]`

	if err := os.WriteFile(filepath.Join(dir, "stations.json"), []byte(stationsJSON), 0o644); err != nil {
		t.Fatalf("write stations.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schedules.json"), []byte(schedulesJSON), 0o644); err != nil {
		t.Fatalf("write schedules.json: %v", err)
	}

	db := setupBackfillTestDB(t)
	if err := db.Create(&models.Station{UID: "old", ID: "OLD", Name: "Old Station", Type: "KRL"}).Error; err != nil {
		t.Fatalf("seed station: %v", err)
	}
	if err := db.Create(&models.Schedule{ID: "old-1", TrainID: "old", Line: "old", Route: "old", StationID: "OLD"}).Error; err != nil {
		t.Fatalf("seed schedule: %v", err)
	}

	if err := backfillFromDataForce(db, dir); err != nil {
		t.Fatalf("backfillFromDataForce() error = %v", err)
	}

	var stationCount int64
	var scheduleCount int64
	_ = db.Model(&models.Station{}).Count(&stationCount)
	_ = db.Model(&models.Schedule{}).Count(&scheduleCount)
	if stationCount < 2 {
		t.Fatalf("expected forced station backfill to add data, got stations=%d", stationCount)
	}
	if scheduleCount < 2 {
		t.Fatalf("expected forced schedule backfill to add data, got schedules=%d", scheduleCount)
	}
}

func TestResolveBackfillDataDir(t *testing.T) {
	t.Run("uses explicit data directory when provided", func(t *testing.T) {
		got := resolveBackfillDataDir("/tmp/custom-data")
		if got != "/tmp/custom-data" {
			t.Fatalf("resolveBackfillDataDir() = %q, want %q", got, "/tmp/custom-data")
		}
	})

	t.Run("discovers data directory from ancestor of cwd", func(t *testing.T) {
		root := t.TempDir()
		dataDir := filepath.Join(root, "data")
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			t.Fatalf("mkdir data dir: %v", err)
		}
		deepDir := filepath.Join(root, "cmd", "api")
		if err := os.MkdirAll(deepDir, 0o755); err != nil {
			t.Fatalf("mkdir nested dir: %v", err)
		}
		t.Chdir(deepDir)

		got := resolveBackfillDataDir("")
		gotAbs, err := filepath.Abs(got)
		if err != nil {
			t.Fatalf("abs path: %v", err)
		}
		wantAbs, err := filepath.Abs(dataDir)
		if err != nil {
			t.Fatalf("abs expected path: %v", err)
		}
		if gotAbs != wantAbs {
			t.Fatalf("resolveBackfillDataDir() = %q, want %q", gotAbs, wantAbs)
		}
	})

	t.Run("falls back to default when no data directory is found", func(t *testing.T) {
		empty := t.TempDir()
		t.Chdir(empty)
		got := resolveBackfillDataDir("")
		if !strings.HasSuffix(filepath.Clean(got), filepath.Clean(defaultBackfillDataDir)) {
			t.Fatalf("resolveBackfillDataDir() = %q, expected fallback to %q", got, defaultBackfillDataDir)
		}
	})
}
