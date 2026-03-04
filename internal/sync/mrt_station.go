package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/comuline/api/internal/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const mrtStationsURL = "https://jakartamrt.co.id/id/val/stasiuns"

// mrtStationRaw represents the raw station data from the Jakarta MRT API.
type mrtStationRaw struct {
	NID            string `json:"nid"`
	Title          string `json:"title"`
	Catatan        string `json:"catatan"`
	IsBig          int    `json:"isbig"`
	Path           string `json:"path"`
	PetaLokalitas  string `json:"peta_lokalitas"`
	Banner         string `json:"banner"`
	Urutan         int    `json:"urutan"`
}

// mrtStationsResponse represents the API response from jakartamrt.co.id.
type mrtStationsResponse struct {
	Status  string          `json:"status"`
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

// MRTStationInfo holds station metadata used for building schedules.
type MRTStationInfo struct {
	NID    string
	ID     string // e.g. "LBB", "FTM"
	Name   string
	Order  int
}

// mrtStationDefinitions maps the 13 MRT North-South Line stations with their IDs.
// Station IDs follow a convention similar to KRL station IDs for consistency.
var mrtStationDefinitions = []MRTStationInfo{
	{NID: "20", ID: "LBB", Name: "Lebak Bulus Grab", Order: 1},
	{NID: "21", ID: "FTM", Name: "Fatmawati Indomaret", Order: 2},
	{NID: "22", ID: "CPR", Name: "Cipete Raya", Order: 3},
	{NID: "23", ID: "HJN", Name: "Haji Nawi", Order: 4},
	{NID: "24", ID: "BLA", Name: "Blok A", Order: 5},
	{NID: "25", ID: "BLM", Name: "Blok M BCA", Order: 6},
	{NID: "26", ID: "ASN", Name: "ASEAN", Order: 7},
	{NID: "27", ID: "SNY", Name: "Senayan Mastercard", Order: 8},
	{NID: "28", ID: "IST", Name: "Istora Mandiri", Order: 9},
	{NID: "29", ID: "BNH", Name: "Bendungan Hilir", Order: 10},
	{NID: "30", ID: "STB", Name: "Setiabudi Astra", Order: 11},
	{NID: "38", ID: "DKA", Name: "Dukuh Atas BNI", Order: 12},
	{NID: "31", ID: "BHI", Name: "Bundaran HI", Order: 13},
}

// BuildMRTStations returns the list of MRT stations as model objects.
func BuildMRTStations() []models.Station {
	stations := make([]models.Station, len(mrtStationDefinitions))
	for i, def := range mrtStationDefinitions {
		metadata, _ := json.Marshal(map[string]any{
			"nid":    def.NID,
			"order":  def.Order,
			"origin": map[string]string{"color": "#DD0067"},
		})
		uid := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(def.Name), " ", "-"))
		stations[i] = models.Station{
			UID:      uid,
			ID:       def.ID,
			Name:     def.Name,
			Type:     "MRT",
			Metadata: datatypes.JSON(metadata),
		}
	}
	return stations
}

// SyncMRTStations upserts MRT station data into the database.
func SyncMRTStations(db *gorm.DB) error {
	log.Println("syncing MRT stations...")

	stations := BuildMRTStations()

	for i := range stations {
		if err := db.Save(&stations[i]).Error; err != nil {
			log.Printf("error saving MRT station %s: %v", stations[i].ID, err)
		}
	}

	log.Printf("MRT station sync complete: %d stations processed", len(stations))
	return nil
}

// FetchMRTStationsFromAPI fetches station data from the Jakarta MRT API.
// This is used to verify/enrich station data but the core station list is hardcoded
// since the API data can be inconsistent.
func FetchMRTStationsFromAPI() ([]mrtStationRaw, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(mrtStationsURL)
	if err != nil {
		return nil, fmt.Errorf("fetching MRT stations: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading MRT stations response: %w", err)
	}

	// The API can return either a direct array or a wrapped response.
	var stations []mrtStationRaw
	if err := json.Unmarshal(body, &stations); err != nil {
		// Try wrapped response format.
		var wrapped mrtStationsResponse
		if err2 := json.Unmarshal(body, &wrapped); err2 != nil {
			return nil, fmt.Errorf("parsing MRT stations response: %w", err)
		}
		if err2 := json.Unmarshal(wrapped.Data, &stations); err2 != nil {
			return nil, fmt.Errorf("parsing MRT stations data: %w", err2)
		}
	}

	return stations, nil
}
