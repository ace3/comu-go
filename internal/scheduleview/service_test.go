package scheduleview

import (
	"context"
	"testing"
	"time"

	"github.com/comu/api/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupScheduleViewDB(t *testing.T) *gorm.DB {
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

func seedKRLData(t *testing.T, db *gorm.DB, date string) {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	ts := func(hhmm string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, loc)
		if err != nil {
			t.Fatalf("parse %s: %v", hhmm, err)
		}
		return v
	}

	stations := []models.Station{
		{UID: "mri", ID: "MRI", Name: "Manggarai", Type: "KRL"},
		{UID: "jakk", ID: "JAKK", Name: "Jakarta Kota", Type: "KRL"},
	}
	for _, st := range stations {
		if err := db.Create(&st).Error; err != nil {
			t.Fatalf("seed station: %v", err)
		}
	}
	schedules := []models.Schedule{
		{ID: "1", TrainID: "T1", Line: "KRL", Route: "R", OriginID: "A", DestinationID: "B", StationID: "MRI", DepartsAt: ts("09:00"), ArrivesAt: ts("09:10")},
		{ID: "2", TrainID: "T2", Line: "KRL", Route: "R", OriginID: "A", DestinationID: "B", StationID: "MRI", DepartsAt: ts("10:00"), ArrivesAt: ts("10:10")},
		{ID: "3", TrainID: "T3", Line: "KRL", Route: "R", OriginID: "A", DestinationID: "B", StationID: "JAKK", DepartsAt: ts("09:15"), ArrivesAt: ts("09:25")},
	}
	for _, s := range schedules {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("seed schedule: %v", err)
		}
	}
}

func TestProjectWindow_ProjectsFromLatestSnapshotDate(t *testing.T) {
	db := setupScheduleViewDB(t)
	seedKRLData(t, db, "2026-03-05")
	svc := New(db)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	at, _ := time.ParseInLocation("2006-01-02 15:04", "2026-03-08 10:00", loc)
	rows, meta, err := svc.ProjectWindow(context.Background(), []string{"MRI", "JAKK"}, at, 60)
	if err != nil {
		t.Fatalf("ProjectWindow() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows len = %d, want 3", len(rows))
	}
	if !meta.Projected {
		t.Fatalf("expected projected=true")
	}
	if meta.SnapshotDateWIB != "2026-03-05" {
		t.Fatalf("snapshot_date_wib = %s", meta.SnapshotDateWIB)
	}
	if rows[0].DepartsAt.Format("2006-01-02") != "2026-03-08" {
		t.Fatalf("departs date = %s, want 2026-03-08", rows[0].DepartsAt.Format("2006-01-02"))
	}
}

func TestProjectStationPage_ProjectsAndCountsSnapshotRows(t *testing.T) {
	db := setupScheduleViewDB(t)
	seedKRLData(t, db, "2026-03-03")
	svc := New(db)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	at, _ := time.ParseInLocation("2006-01-02 15:04", "2026-03-09 10:00", loc)
	rows, total, meta, err := svc.ProjectStationPage(context.Background(), "MRI", 1, 100, at)
	if err != nil {
		t.Fatalf("ProjectStationPage() error = %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if meta.SnapshotAgeDays < 0 {
		t.Fatalf("snapshot_age_days = %d, want >=0", meta.SnapshotAgeDays)
	}
	if rows[0].DepartsAt.Format("2006-01-02") != "2026-03-09" {
		t.Fatalf("departs date = %s, want 2026-03-09", rows[0].DepartsAt.Format("2006-01-02"))
	}
}

func TestProjectStationRange_ProjectsFilteredRowsWithinForwardWindow(t *testing.T) {
	db := setupScheduleViewDB(t)
	seedKRLData(t, db, "2026-03-03")
	svc := New(db)

	loc, _ := time.LoadLocation("Asia/Jakarta")
	start, _ := time.ParseInLocation("2006-01-02 15:04", "2026-03-09 09:30", loc)
	end := start.Add(60 * time.Minute)

	rows, total, meta, err := svc.ProjectStationRange(context.Background(), "MRI", start, end, 1, 100)
	if err != nil {
		t.Fatalf("ProjectStationRange() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	if !meta.Projected {
		t.Fatalf("expected projected=true")
	}
	if rows[0].TrainID != "T2" {
		t.Fatalf("train_id = %s, want T2", rows[0].TrainID)
	}
	if rows[0].DepartsAt.Format(time.RFC3339) != "2026-03-09T10:00:00+07:00" {
		t.Fatalf("departs_at = %s", rows[0].DepartsAt.Format(time.RFC3339))
	}
}
