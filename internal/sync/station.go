package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type krlStation struct {
	StaID    string `json:"sta_id"`
	StaName  string `json:"sta_name"`
	FgEnable int    `json:"fg_enable"`
	Daop     int    `json:"daop"`
	GroupWil int    `json:"group_wil"`
}

type krlStationResponse struct {
	Status int          `json:"status"`
	Data   []krlStation `json:"data"`
}

var hardcodedStations = []models.Station{
	{
		UID:      "bandara-soekarno-hatta",
		ID:       "BANDARA",
		Name:     "Bandara Soekarno Hatta",
		Type:     "KRL",
		Metadata: datatypes.JSON(`{"daop":1,"fg_enable":1}`),
	},
	{
		UID:      "cikampek",
		ID:       "CKP",
		Name:     "Cikampek",
		Type:     "KRL",
		Metadata: datatypes.JSON(`{"daop":1,"fg_enable":1}`),
	},
	{
		UID:      "purwakarta",
		ID:       "PWK",
		Name:     "Purwakarta",
		Type:     "KRL",
		Metadata: datatypes.JSON(`{"daop":2,"fg_enable":1}`),
	},
}

// SyncStations fetches stations from the KRL API and upserts them into the database.
func SyncStations(cfg *config.Config, db *gorm.DB) error {
	slog.Info("fetching stations from KRL API")

	req, err := http.NewRequest("GET", "https://api-partner.krl.co.id/krl-webs/v1/krl-station", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	setKRLHeaders(req, cfg.KAIAuthToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := fetchWithRetry(client, req, 3)
	if err != nil {
		return fmt.Errorf("fetching stations: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	var krlResp krlStationResponse
	if err := json.Unmarshal(body, &krlResp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	var stations []models.Station
	for _, s := range krlResp.Data {
		if strings.Contains(s.StaID, "WIL") {
			continue
		}

		metadata, _ := json.Marshal(map[string]any{
			"daop":      s.Daop,
			"fg_enable": s.FgEnable,
		})

		uid := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s.StaName), " ", "-"))
		stations = append(stations, models.Station{
			UID:      uid,
			ID:       s.StaID,
			Name:     TitleCase(s.StaName),
			Type:     "KRL",
			Metadata: datatypes.JSON(metadata),
		})
	}

	stations = append(stations, hardcodedStations...)

	slog.Info("upserting stations", "count", len(stations))
	for i := range stations {
		if err := db.Save(&stations[i]).Error; err != nil {
			slog.Error("error saving station", "station_id", stations[i].ID, "error", err)
		}
	}

	slog.Info("station sync complete", "count", len(stations))
	return nil
}

// TitleCase converts "MANGGARAI BESAR" → "Manggarai Besar".
func TitleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(string(w[0])) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
