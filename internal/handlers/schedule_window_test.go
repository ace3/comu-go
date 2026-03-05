package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/comu/api/internal/models"
	"github.com/comu/api/internal/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type windowTestResponse struct {
	Metadata response.Metadata `json:"metadata"`
	Data     json.RawMessage   `json:"data"`
}

type windowData struct {
	AtWIB          string              `json:"at_wib"`
	WindowStartWIB string              `json:"window_start_wib"`
	WindowEndWIB   string              `json:"window_end_wib"`
	Stations       []windowStationData `json:"stations"`
}

type windowStationData struct {
	StationID string               `json:"station_id"`
	Schedules []windowScheduleData `json:"schedules"`
}

type windowScheduleData struct {
	TrainID   string `json:"train_id"`
	DepartsAt string `json:"departs_at"`
}

func seedWindowSchedules(t *testing.T, db *gorm.DB, loc *time.Location) {
	t.Helper()

	mk := func(id, stationID, trainID, hhmm string) models.Schedule {
		ts, err := time.ParseInLocation("2006-01-02 15:04", "2026-03-05 "+hhmm, loc)
		if err != nil {
			t.Fatalf("parse time %s: %v", hhmm, err)
		}
		return models.Schedule{
			ID:            id,
			TrainID:       trainID,
			Line:          "KRL",
			Route:         "ROUTE",
			OriginID:      "ORG",
			DestinationID: "DST",
			StationID:     stationID,
			DepartsAt:     ts,
			ArrivesAt:     ts.Add(10 * time.Minute),
		}
	}

	schedules := []models.Schedule{
		mk("mri-0859", "MRI", "T0859", "08:59"),
		mk("mri-0900", "MRI", "T0900", "09:00"),
		mk("mri-1030", "MRI", "T1030", "10:30"),
		mk("mri-1100", "MRI", "T1100", "11:00"),
		mk("mri-1101", "MRI", "T1101", "11:01"),
		mk("jakk-0915", "JAKK", "T0915", "09:15"),
	}

	for _, s := range schedules {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("failed to seed schedule %s: %v", s.ID, err)
		}
	}
}

func TestScheduleHandler_GetScheduleWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	h := NewScheduleHandler(db, nil)

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	seedWindowSchedules(t, db, loc)

	t.Run("returns schedules for requested stations in WIB +/- 1h window", func(t *testing.T) {
		at := "2026-03-05T10:00:00+07:00"
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/schedule/window?station_ids=MRI,JAKK&window_minutes=60&at="+url.QueryEscape(at), nil)

		h.GetScheduleWindow(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200", w.Code)
		}

		var resp windowTestResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		if !resp.Metadata.Success {
			t.Fatalf("expected success=true")
		}

		var data windowData
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("parse data: %v", err)
		}

		if data.AtWIB != at {
			t.Errorf("at_wib = %q, expected %q", data.AtWIB, at)
		}
		if data.WindowStartWIB != "2026-03-05T09:00:00+07:00" {
			t.Errorf("window_start_wib = %q", data.WindowStartWIB)
		}
		if data.WindowEndWIB != "2026-03-05T11:00:00+07:00" {
			t.Errorf("window_end_wib = %q", data.WindowEndWIB)
		}
		if len(data.Stations) != 2 {
			t.Fatalf("stations len = %d, expected 2", len(data.Stations))
		}

		if data.Stations[0].StationID != "MRI" {
			t.Fatalf("stations[0] = %q, expected MRI", data.Stations[0].StationID)
		}
		if len(data.Stations[0].Schedules) != 3 {
			t.Fatalf("MRI schedules len = %d, expected 3", len(data.Stations[0].Schedules))
		}
		if data.Stations[0].Schedules[0].TrainID != "T0900" {
			t.Errorf("first MRI train = %q, expected T0900", data.Stations[0].Schedules[0].TrainID)
		}
		if data.Stations[0].Schedules[2].TrainID != "T1100" {
			t.Errorf("last MRI train = %q, expected T1100", data.Stations[0].Schedules[2].TrainID)
		}

		if data.Stations[1].StationID != "JAKK" {
			t.Fatalf("stations[1] = %q, expected JAKK", data.Stations[1].StationID)
		}
		if len(data.Stations[1].Schedules) != 1 {
			t.Fatalf("JAKK schedules len = %d, expected 1", len(data.Stations[1].Schedules))
		}
	})

	t.Run("returns empty schedule list for station with no data in range", func(t *testing.T) {
		at := "2026-03-05T10:00:00+07:00"
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/schedule/window?station_ids=BOO&window_minutes=60&at="+url.QueryEscape(at), nil)

		h.GetScheduleWindow(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200", w.Code)
		}

		var resp windowTestResponse
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		var data windowData
		_ = json.Unmarshal(resp.Data, &data)
		if len(data.Stations) != 1 || data.Stations[0].StationID != "BOO" {
			t.Fatalf("unexpected stations response: %+v", data.Stations)
		}
		if len(data.Stations[0].Schedules) != 0 {
			t.Fatalf("expected 0 schedules, got %d", len(data.Stations[0].Schedules))
		}
	})

	t.Run("returns 400 when station_ids is missing", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/schedule/window?window_minutes=60", nil)

		h.GetScheduleWindow(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, expected 400", w.Code)
		}
	})

	t.Run("returns 400 when more than five station IDs", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/schedule/window?station_ids=A,B,C,D,E,F", nil)

		h.GetScheduleWindow(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, expected 400", w.Code)
		}
	})

	t.Run("returns 400 for invalid window_minutes", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/schedule/window?station_ids=MRI&window_minutes=abc", nil)

		h.GetScheduleWindow(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, expected 400", w.Code)
		}
	})

	t.Run("returns 400 for invalid at", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/schedule/window?station_ids=MRI&at=bad-time", nil)

		h.GetScheduleWindow(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, expected 400", w.Code)
		}
	})
}
