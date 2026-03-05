package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type krlSchedule struct {
	TrainID   string `json:"train_id"`
	KaName    string `json:"ka_name"`
	RouteName string `json:"route_name"`
	Dest      string `json:"dest"`
	TimeEst   string `json:"time_est"`
	DestTime  string `json:"dest_time"`
	OriginID  string `json:"origin_id"`
	DestID    string `json:"dest_id"`
	StationID string `json:"station_id"`
}

type krlScheduleResponse struct {
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"` // can be [] or "No data" string
}

const scheduleSyncParallelism = 5

var syncStationSchedulesFunc = syncStationSchedules

// SyncSchedules loads all stations and fetches schedules with bounded parallelism.
func SyncSchedules(cfg *config.Config, db *gorm.DB) error {
	var stations []models.Station
	if err := db.Where("type = ? OR type = '' OR type IS NULL", "KRL").Find(&stations).Error; err != nil {
		return fmt.Errorf("loading stations: %w", err)
	}

	slog.Info("syncing schedules", "stations", len(stations), "parallelism", scheduleSyncParallelism)

	jobs := make(chan string, len(stations))
	var wg sync.WaitGroup

	workerCount := scheduleSyncParallelism
	if len(stations) < workerCount {
		workerCount = len(stations)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for stationID := range jobs {
				if err := syncStationSchedulesFunc(cfg, db, stationID); err != nil {
					slog.Error("error syncing station schedule", "station_id", stationID, "error", err)
				}
			}
		}()
	}

	for _, station := range stations {
		jobs <- station.ID
	}
	close(jobs)
	wg.Wait()

	slog.Info("schedule sync complete")
	return nil
}

func syncStationSchedules(cfg *config.Config, db *gorm.DB, stationID string) error {
	url := fmt.Sprintf(
		"https://api-partner.krl.co.id/krl-webs/v1/schedules?stationid=%s&timefrom=00:00&timeto=23:59",
		stationID,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	setKRLHeaders(req, cfg.Token())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := fetchWithRetry(client, req, 3)
	if err != nil {
		return fmt.Errorf("fetching schedule for %s: %w", stationID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var krlResp krlScheduleResponse
	if err := json.Unmarshal(body, &krlResp); err != nil {
		return fmt.Errorf("parsing response for %s: %w", stationID, err)
	}

	// API returns "No data" string instead of [] when station has no schedules.
	var items []krlSchedule
	if err := json.Unmarshal(krlResp.Data, &items); err != nil || len(items) == 0 {
		return nil
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	today := time.Now().In(loc)

	var schedules []models.Schedule
	for _, s := range items {
		departsAt := parseScheduleTime(s.TimeEst, today, loc)
		arrivesAt := parseScheduleTime(s.DestTime, today, loc)

		meta, _ := json.Marshal(map[string]any{
			"ka_name": fixName(s.KaName),
			"dest":    fixName(s.Dest),
		})

		schedules = append(schedules, models.Schedule{
			ID:            fmt.Sprintf("%s-%s-%s", stationID, s.TrainID, departsAt.Format("1504")),
			TrainID:       s.TrainID,
			Line:          fixName(s.KaName),
			Route:         s.RouteName,
			OriginID:      s.OriginID,
			DestinationID: s.DestID,
			StationID:     stationID,
			DepartsAt:     departsAt,
			ArrivesAt:     arrivesAt,
			Metadata:      datatypes.JSON(meta),
		})
	}

	result := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"train_id", "line", "route", "origin_id", "destination_id",
			"station_id", "departs_at", "arrives_at", "metadata", "updated_at",
		}),
	}).Create(&schedules)

	if result.Error != nil {
		return fmt.Errorf("upserting schedules for %s: %w", stationID, result.Error)
	}

	slog.Info("synced schedules for station", "count", len(schedules), "station_id", stationID)
	return nil
}

// parseScheduleTime parses "HH:MM:SS" or "HH:MM" into a time.Time anchored to today.
func parseScheduleTime(timeStr string, today time.Time, loc *time.Location) time.Time {
	parts := strings.Split(strings.TrimSpace(timeStr), ":")
	if len(parts) < 2 {
		return today
	}

	var h, m, s int
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	if len(parts) > 2 {
		fmt.Sscanf(parts[2], "%d", &s)
	}

	return time.Date(today.Year(), today.Month(), today.Day(), h, m, s, 0, loc)
}

// fixName normalizes KRL train/route names to title case.
func fixName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	return TitleCase(name)
}
