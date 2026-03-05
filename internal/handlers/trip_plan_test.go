package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/comu/api/internal/models"
	"github.com/comu/api/internal/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tripPlanTestResponse struct {
	Metadata response.Metadata `json:"metadata"`
	Data     tripPlanData      `json:"data"`
}

type tripPlanData struct {
	Options []struct {
		DepartAt string `json:"departAt"`
		ArriveAt string `json:"arriveAt"`
		Legs     []struct {
			TrainID string `json:"trainId"`
			From    string `json:"from"`
			To      string `json:"to"`
		} `json:"legs"`
	} `json:"options"`
}

func seedTripPlanSchedules(t *testing.T, db *gorm.DB) {
	t.Helper()

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	mkTime := func(hhmm string) time.Time {
		ts, parseErr := time.ParseInLocation("2006-01-02 15:04", "2026-03-05 "+hhmm, loc)
		if parseErr != nil {
			t.Fatalf("parse time %s: %v", hhmm, parseErr)
		}
		return ts
	}

	rows := []models.Schedule{
		{ID: "a-rw", TrainID: "A1", Line: "Commuter Line Tangerang", Route: "RW-DU", OriginID: "RW", DestinationID: "DU", StationID: "RW", DepartsAt: mkTime("15:52"), ArrivesAt: mkTime("15:52")},
		{ID: "a-du", TrainID: "A1", Line: "Commuter Line Tangerang", Route: "RW-DU", OriginID: "RW", DestinationID: "DU", StationID: "DU", DepartsAt: mkTime("16:08"), ArrivesAt: mkTime("16:08")},

		{ID: "b-rw", TrainID: "A2", Line: "Commuter Line Tangerang", Route: "RW-DU", OriginID: "RW", DestinationID: "DU", StationID: "RW", DepartsAt: mkTime("15:35"), ArrivesAt: mkTime("15:35")},
		{ID: "b-du", TrainID: "A2", Line: "Commuter Line Tangerang", Route: "RW-DU", OriginID: "RW", DestinationID: "DU", StationID: "DU", DepartsAt: mkTime("15:51"), ArrivesAt: mkTime("15:51")},

		{ID: "c-du", TrainID: "C1", Line: "Commuter Line Cikarang", Route: "DU-SUDB", OriginID: "DU", DestinationID: "SUDB", StationID: "DU", DepartsAt: mkTime("16:17"), ArrivesAt: mkTime("16:17")},
		{ID: "c-sudb", TrainID: "C1", Line: "Commuter Line Cikarang", Route: "DU-SUDB", OriginID: "DU", DestinationID: "SUDB", StationID: "SUDB", DepartsAt: mkTime("17:05"), ArrivesAt: mkTime("17:05")},

		{ID: "d-du", TrainID: "C2", Line: "Commuter Line Cikarang", Route: "DU-SUDB", OriginID: "DU", DestinationID: "SUDB", StationID: "DU", DepartsAt: mkTime("16:08"), ArrivesAt: mkTime("16:08")},
		{ID: "d-sudb", TrainID: "C2", Line: "Commuter Line Cikarang", Route: "DU-SUDB", OriginID: "DU", DestinationID: "SUDB", StationID: "SUDB", DepartsAt: mkTime("17:20"), ArrivesAt: mkTime("17:20")},
	}

	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed schedule %s: %v", row.ID, err)
		}
	}
}

func TestTripPlanHandler_GetTripPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	seedTripPlanSchedules(t, db)
	h := NewTripPlanHandler(db, nil)

	t.Run("returns one request planned options and removes dominated", func(t *testing.T) {
		body := map[string]any{
			"from_id":        "RW",
			"to_id":          "SUDB",
			"at":             "2026-03-05T15:51:00+07:00",
			"window_minutes": 60,
			"max_results":    8,
		}
		raw, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/trip-plan", bytes.NewReader(raw))
		c.Request.Header.Set("Content-Type", "application/json")

		h.GetTripPlan(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200, body=%s", w.Code, w.Body.String())
		}
		var resp tripPlanTestResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		if !resp.Metadata.Success {
			t.Fatalf("expected success=true")
		}
		if len(resp.Data.Options) == 0 {
			t.Fatalf("expected options > 0")
		}
		if resp.Data.Options[0].DepartAt < "2026-03-05T15:51:00+07:00" {
			t.Fatalf("first departAt %s is earlier than requested at", resp.Data.Options[0].DepartAt)
		}
		for _, opt := range resp.Data.Options {
			if len(opt.Legs) == 2 && opt.Legs[0].TrainID == "A2" && opt.Legs[1].TrainID == "C2" {
				t.Fatalf("dominated option A2->C2 should not be present")
			}
		}
	})

	t.Run("returns 400 for invalid payload", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/trip-plan", bytes.NewReader([]byte(`{}`)))
		c.Request.Header.Set("Content-Type", "application/json")

		h.GetTripPlan(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, expected 400", w.Code)
		}
	})
}
