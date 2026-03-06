package scheduleview

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/comu/api/internal/models"
	"gorm.io/gorm"
)

type ProjectionMeta struct {
	Projected       bool
	SnapshotDateWIB string
	SnapshotAgeDays int
	HasSnapshot     bool
}

type Service struct {
	db  *gorm.DB
	loc *time.Location
}

func New(db *gorm.DB) *Service {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	return &Service{db: db, loc: loc}
}

func (s *Service) ResolveSnapshot(ctx context.Context) (time.Time, ProjectionMeta, error) {
	var latestRaw sql.NullString
	err := s.db.WithContext(ctx).
		Table("schedules as sc").
		Select("MAX(sc.departs_at)").
		Joins("JOIN stations st ON st.id = sc.station_id").
		Where("st.type = ? OR st.type = '' OR st.type IS NULL", "KRL").
		Scan(&latestRaw).Error
	if err != nil {
		return time.Time{}, ProjectionMeta{}, err
	}
	if !latestRaw.Valid || strings.TrimSpace(latestRaw.String) == "" {
		if err := s.db.WithContext(ctx).
			Table("schedules").
			Select("MAX(departs_at)").
			Scan(&latestRaw).Error; err != nil {
			return time.Time{}, ProjectionMeta{}, err
		}
	}
	if !latestRaw.Valid || strings.TrimSpace(latestRaw.String) == "" {
		return time.Time{}, ProjectionMeta{}, nil
	}

	latest, err := parseDBTime(latestRaw.String, s.loc)
	if err != nil {
		return time.Time{}, ProjectionMeta{}, err
	}
	snapshotAt := latest.In(s.loc)
	snapshotDate := time.Date(snapshotAt.Year(), snapshotAt.Month(), snapshotAt.Day(), 0, 0, 0, 0, s.loc)
	now := time.Now().In(s.loc)
	ageDays := int(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc).Sub(snapshotDate).Hours() / 24)
	if ageDays < 0 {
		ageDays = 0
	}

	return snapshotDate, ProjectionMeta{
		Projected:       false,
		SnapshotDateWIB: snapshotDate.Format("2006-01-02"),
		SnapshotAgeDays: ageDays,
		HasSnapshot:     true,
	}, nil
}

func (s *Service) ProjectStationPage(ctx context.Context, stationID string, page, limit int, targetAtWIB time.Time) ([]models.Schedule, int64, ProjectionMeta, error) {
	snapshotDate, meta, err := s.ResolveSnapshot(ctx)
	if err != nil {
		return nil, 0, ProjectionMeta{}, err
	}
	if !meta.HasSnapshot {
		return []models.Schedule{}, 0, meta, nil
	}

	stationID = strings.ToUpper(strings.TrimSpace(stationID))
	start := dayStart(snapshotDate, s.loc)
	end := start.Add(24 * time.Hour)

	var total int64
	if err := s.db.WithContext(ctx).
		Model(&models.Schedule{}).
		Where("station_id = ? AND departs_at >= ? AND departs_at < ?", stationID, start, end).
		Count(&total).Error; err != nil {
		return nil, 0, ProjectionMeta{}, err
	}

	var rows []models.Schedule
	offset := (page - 1) * limit
	if err := s.db.WithContext(ctx).
		Where("station_id = ? AND departs_at >= ? AND departs_at < ?", stationID, start, end).
		Order("departs_at asc").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, ProjectionMeta{}, err
	}

	targetDate := dayStart(targetAtWIB.In(s.loc), s.loc)
	projected := !sameDay(snapshotDate, targetDate, s.loc)
	meta.Projected = projected
	return shiftRowsToDate(rows, targetDate, s.loc), total, meta, nil
}

func (s *Service) ProjectStationRange(ctx context.Context, stationID string, targetStartWIB, targetEndWIB time.Time, page, limit int) ([]models.Schedule, int64, ProjectionMeta, error) {
	snapshotDate, meta, err := s.ResolveSnapshot(ctx)
	if err != nil {
		return nil, 0, ProjectionMeta{}, err
	}
	if !meta.HasSnapshot {
		return []models.Schedule{}, 0, meta, nil
	}

	stationID = strings.ToUpper(strings.TrimSpace(stationID))
	targetStartWIB = targetStartWIB.In(s.loc)
	targetEndWIB = targetEndWIB.In(s.loc)
	if !targetEndWIB.After(targetStartWIB) {
		return []models.Schedule{}, 0, meta, nil
	}

	start := toClockOnDate(targetStartWIB, snapshotDate, s.loc)
	end := toClockOnDate(targetEndWIB, snapshotDate, s.loc)

	var total int64
	if err := s.db.WithContext(ctx).
		Model(&models.Schedule{}).
		Where("station_id = ? AND departs_at >= ? AND departs_at < ?", stationID, start, end).
		Count(&total).Error; err != nil {
		return nil, 0, ProjectionMeta{}, err
	}

	var rows []models.Schedule
	offset := (page - 1) * limit
	if err := s.db.WithContext(ctx).
		Where("station_id = ? AND departs_at >= ? AND departs_at < ?", stationID, start, end).
		Order("departs_at asc").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, ProjectionMeta{}, err
	}

	targetDate := dayStart(targetStartWIB, s.loc)
	meta.Projected = !sameDay(snapshotDate, targetDate, s.loc)
	return shiftRowsToDate(rows, targetDate, s.loc), total, meta, nil
}

func (s *Service) ProjectWindow(ctx context.Context, stationIDs []string, targetAtWIB time.Time, windowMinutes int) ([]models.Schedule, ProjectionMeta, error) {
	snapshotDate, meta, err := s.ResolveSnapshot(ctx)
	if err != nil {
		return nil, ProjectionMeta{}, err
	}
	if !meta.HasSnapshot {
		return []models.Schedule{}, meta, nil
	}

	targetAtWIB = targetAtWIB.In(s.loc)
	window := time.Duration(windowMinutes) * time.Minute
	start := targetAtWIB.Add(-window)
	end := targetAtWIB.Add(window)

	rows, err := s.queryProjectedRange(ctx, stationIDs, start, end, 0, "station_id asc, departs_at asc")
	if err != nil {
		return nil, ProjectionMeta{}, err
	}

	targetDate := dayStart(targetAtWIB, s.loc)
	meta.Projected = !sameDay(snapshotDate, targetDate, s.loc)
	return rows, meta, nil
}

func (s *Service) ProjectForTripPlan(ctx context.Context, stationIDs []string, start, end time.Time, limit int) ([]models.Schedule, ProjectionMeta, error) {
	snapshotDate, meta, err := s.ResolveSnapshot(ctx)
	if err != nil {
		return nil, ProjectionMeta{}, err
	}
	if !meta.HasSnapshot {
		return []models.Schedule{}, meta, nil
	}
	rows, err := s.queryProjectedRange(ctx, stationIDs, start, end, limit, "station_id asc, departs_at asc")
	if err != nil {
		return nil, ProjectionMeta{}, err
	}

	targetDate := dayStart(start.In(s.loc), s.loc)
	meta.Projected = !sameDay(snapshotDate, targetDate, s.loc)
	return rows, meta, nil
}

func (s *Service) ProjectRoutesByTrain(ctx context.Context, trainIDs []string, targetDate time.Time) (map[string][]models.Schedule, ProjectionMeta, error) {
	snapshotDate, meta, err := s.ResolveSnapshot(ctx)
	if err != nil {
		return nil, ProjectionMeta{}, err
	}
	out := make(map[string][]models.Schedule, len(trainIDs))
	if !meta.HasSnapshot || len(trainIDs) == 0 {
		return out, meta, nil
	}

	var rows []models.Schedule
	start := dayStart(snapshotDate, s.loc)
	end := start.Add(24 * time.Hour)
	if err := s.db.WithContext(ctx).
		Where("train_id IN ? AND departs_at >= ? AND departs_at < ?", trainIDs, start, end).
		Order("train_id asc, departs_at asc").
		Find(&rows).Error; err != nil {
		return nil, ProjectionMeta{}, err
	}

	target := dayStart(targetDate.In(s.loc), s.loc)
	projectedRows := shiftRowsToDate(rows, target, s.loc)
	for _, row := range projectedRows {
		id := strings.ToUpper(strings.TrimSpace(row.TrainID))
		out[id] = append(out[id], row)
	}
	meta.Projected = !sameDay(snapshotDate, target, s.loc)
	return out, meta, nil
}

func (s *Service) queryProjectedRange(ctx context.Context, stationIDs []string, targetStart, targetEnd time.Time, limit int, orderBy string) ([]models.Schedule, error) {
	if len(stationIDs) == 0 {
		return []models.Schedule{}, nil
	}
	snapshotDate, meta, err := s.ResolveSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if !meta.HasSnapshot {
		return []models.Schedule{}, nil
	}

	normalized := make([]string, 0, len(stationIDs))
	for _, id := range stationIDs {
		id = strings.ToUpper(strings.TrimSpace(id))
		if id != "" {
			normalized = append(normalized, id)
		}
	}
	if len(normalized) == 0 {
		return []models.Schedule{}, nil
	}

	startTOD := toClockOnDate(targetStart.In(s.loc), snapshotDate, s.loc)
	endTOD := toClockOnDate(targetEnd.In(s.loc), snapshotDate, s.loc)
	fullStart := dayStart(snapshotDate, s.loc)
	fullEnd := fullStart.Add(24 * time.Hour)

	var rows []models.Schedule
	q := s.db.WithContext(ctx).Where("station_id IN ?", normalized)
	if crossesMidnight(targetStart.In(s.loc), targetEnd.In(s.loc), s.loc) {
		q = q.Where("(departs_at >= ? AND departs_at < ?) OR (departs_at >= ? AND departs_at <= ?)", startTOD, fullEnd, fullStart, endTOD)
	} else {
		q = q.Where("departs_at BETWEEN ? AND ?", startTOD, endTOD)
	}
	if orderBy != "" {
		q = q.Order(orderBy)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	projectedDate := dayStart(targetStart.In(s.loc), s.loc)
	return shiftRowsToDate(rows, projectedDate, s.loc), nil
}

func dayStart(t time.Time, loc *time.Location) time.Time {
	v := t.In(loc)
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, loc)
}

func sameDay(a, b time.Time, loc *time.Location) bool {
	av := a.In(loc)
	bv := b.In(loc)
	return av.Year() == bv.Year() && av.Month() == bv.Month() && av.Day() == bv.Day()
}

func toClockOnDate(source, date time.Time, loc *time.Location) time.Time {
	v := source.In(loc)
	d := date.In(loc)
	return time.Date(d.Year(), d.Month(), d.Day(), v.Hour(), v.Minute(), v.Second(), v.Nanosecond(), loc)
}

func shiftRowsToDate(rows []models.Schedule, targetDate time.Time, loc *time.Location) []models.Schedule {
	projected := make([]models.Schedule, 0, len(rows))
	for _, row := range rows {
		p := row
		p.DepartsAt = toClockOnDate(row.DepartsAt, targetDate, loc)
		p.ArrivesAt = toClockOnDate(row.ArrivesAt, targetDate, loc)
		projected = append(projected, p)
	}
	return projected
}

func crossesMidnight(start, end time.Time, loc *time.Location) bool {
	s := start.In(loc)
	e := end.In(loc)
	if e.Before(s) {
		return true
	}
	return !sameDay(s, e, loc)
}

func (m ProjectionMeta) String() string {
	return fmt.Sprintf("projected=%t snapshot_date=%s age_days=%d", m.Projected, m.SnapshotDateWIB, m.SnapshotAgeDays)
}

func parseDBTime(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999-0700",
		"2006-01-02 15:04:05-0700",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts, nil
		}
		if ts, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse snapshot timestamp %q", raw)
}
