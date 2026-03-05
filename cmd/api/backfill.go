package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/comu/api/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultBackfillDataDir = "data"

func backfillFromDataIfEmpty(db *gorm.DB, dataDir string) error {
	var stationCount int64
	if err := db.Model(&models.Station{}).Count(&stationCount).Error; err != nil {
		return fmt.Errorf("counting stations: %w", err)
	}

	var scheduleCount int64
	if err := db.Model(&models.Schedule{}).Count(&scheduleCount).Error; err != nil {
		return fmt.Errorf("counting schedules: %w", err)
	}

	if stationCount > 0 && scheduleCount > 0 {
		return nil
	}

	dataDir = resolveBackfillDataDir(dataDir)

	if stationCount == 0 {
		if err := backfillStationsFromFile(db, filepath.Join(dataDir, "stations.json")); err != nil {
			return err
		}
	}

	if scheduleCount == 0 {
		if err := backfillSchedulesFromFile(db, filepath.Join(dataDir, "schedules.json")); err != nil {
			return err
		}
	}

	return nil
}

type stationDataRow struct {
	UID      string `json:"uid"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Metadata any    `json:"metadata"`
}

type scheduleDataRow struct {
	ID            string `json:"id"`
	TrainID       string `json:"train_id"`
	Line          string `json:"line"`
	Route         string `json:"route"`
	OriginID      string `json:"origin_id"`
	DestinationID string `json:"destination_id"`
	StationID     string `json:"station_id"`
	DepartsAt     string `json:"departs_at"`
	ArrivesAt     string `json:"arrives_at"`
	Metadata      any    `json:"metadata"`
}

func backfillStationsFromFile(db *gorm.DB, path string) error {
	rows, err := loadStationRows(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Warn("stations backfill file not found, skipping", "path", path)
			return nil
		}
		return err
	}

	if len(rows) == 0 {
		slog.Warn("stations backfill file is empty", "path", path)
		return nil
	}

	stations := make([]models.Station, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.UID) == "" || strings.TrimSpace(row.ID) == "" {
			continue
		}
		meta, err := normalizeMetadata(row.Metadata)
		if err != nil {
			return fmt.Errorf("parsing station metadata for %s: %w", row.ID, err)
		}
		typeValue := row.Type
		if strings.TrimSpace(typeValue) == "" {
			typeValue = "KRL"
		}
		stations = append(stations, models.Station{
			UID:      row.UID,
			ID:       strings.ToUpper(row.ID),
			Name:     row.Name,
			Type:     typeValue,
			Metadata: meta,
		})
	}

	for i := range stations {
		if err := db.Save(&stations[i]).Error; err != nil {
			return fmt.Errorf("upserting station %s: %w", stations[i].ID, err)
		}
	}

	slog.Info("stations backfilled from file", "path", path, "count", len(stations))
	return nil
}

func backfillSchedulesFromFile(db *gorm.DB, path string) error {
	rows, err := loadScheduleRows(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Warn("schedules backfill file not found, skipping", "path", path)
			return nil
		}
		return err
	}

	if len(rows) == 0 {
		slog.Warn("schedules backfill file is empty", "path", path)
		return nil
	}

	schedules := make([]models.Schedule, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ID) == "" || strings.TrimSpace(row.StationID) == "" {
			continue
		}

		departsAt, err := parseBackfillTime(row.DepartsAt)
		if err != nil {
			return fmt.Errorf("parsing departs_at for schedule %s: %w", row.ID, err)
		}
		arrivesAt, err := parseBackfillTime(row.ArrivesAt)
		if err != nil {
			return fmt.Errorf("parsing arrives_at for schedule %s: %w", row.ID, err)
		}
		meta, err := normalizeMetadata(row.Metadata)
		if err != nil {
			return fmt.Errorf("parsing schedule metadata for %s: %w", row.ID, err)
		}

		schedules = append(schedules, models.Schedule{
			ID:            row.ID,
			TrainID:       row.TrainID,
			Line:          row.Line,
			Route:         row.Route,
			OriginID:      row.OriginID,
			DestinationID: row.DestinationID,
			StationID:     strings.ToUpper(row.StationID),
			DepartsAt:     departsAt,
			ArrivesAt:     arrivesAt,
			Metadata:      meta,
		})
	}

	if len(schedules) == 0 {
		return nil
	}

	result := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"train_id", "line", "route", "origin_id", "destination_id",
			"station_id", "departs_at", "arrives_at", "metadata", "updated_at",
		}),
	}).CreateInBatches(&schedules, 1000)
	if result.Error != nil {
		return fmt.Errorf("upserting schedules: %w", result.Error)
	}

	slog.Info("schedules backfilled from file", "path", path, "count", len(schedules))
	return nil
}

func loadStationRows(path string) ([]stationDataRow, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading stations backfill file %s: %w", path, err)
	}
	var rows []stationDataRow
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, fmt.Errorf("parsing stations backfill file %s: %w", path, err)
	}
	return rows, nil
}

func loadScheduleRows(path string) ([]scheduleDataRow, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schedules backfill file %s: %w", path, err)
	}
	var rows []scheduleDataRow
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, fmt.Errorf("parsing schedules backfill file %s: %w", path, err)
	}
	return rows, nil
}

func normalizeMetadata(input any) (datatypes.JSON, error) {
	switch v := input.(type) {
	case nil:
		return datatypes.JSON([]byte("{}")), nil
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return datatypes.JSON([]byte("{}")), nil
		}
		if !json.Valid([]byte(raw)) {
			return nil, fmt.Errorf("invalid json string")
		}
		return datatypes.JSON([]byte(raw)), nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return datatypes.JSON(b), nil
	}
}

func parseBackfillTime(value string) (time.Time, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return time.Time{}, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999-07",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05-07",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format: %s", raw)
}

func resolveBackfillDataDir(dataDir string) string {
	if trimmed := strings.TrimSpace(dataDir); trimmed != "" {
		return trimmed
	}

	if found, ok := findDataDirFrom(filepath.Clean(".")); ok {
		return found
	}

	if exe, err := os.Executable(); err == nil {
		if found, ok := findDataDirFrom(filepath.Dir(exe)); ok {
			return found
		}
	}

	candidates := []string{
		defaultBackfillDataDir,
		"./data",
		"/app/data",
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate
		}
	}
	return defaultBackfillDataDir
}

func findDataDirFrom(start string) (string, bool) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(current, "data")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", false
}
