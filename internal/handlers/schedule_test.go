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
)

type paginatedScheduleResponse struct {
	Metadata response.PaginatedMetadata `json:"metadata"`
	Data     []models.Schedule          `json:"data"`
}

func TestScheduleHandler_GetSchedules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	h := NewScheduleHandler(db, nil)

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	seedWindowSchedules(t, db, loc)

	t.Run("filters schedules after departs_after within a bounded forward window", func(t *testing.T) {
		at := "2026-03-08T10:00:00+07:00"
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "station_id", Value: "MRI"}}
		c.Request = httptest.NewRequest(
			http.MethodGet,
			"/v1/schedule/MRI?limit=10&window_minutes=120&departs_after="+url.QueryEscape(at),
			nil,
		)

		h.GetSchedules(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200 body=%s", w.Code, w.Body.String())
		}

		var resp paginatedScheduleResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		if !resp.Metadata.Success {
			t.Fatalf("expected success=true")
		}
		if !resp.Metadata.Projected {
			t.Fatalf("expected projected=true")
		}
		if resp.Metadata.Total != 3 {
			t.Fatalf("total = %d, expected 3", resp.Metadata.Total)
		}
		if len(resp.Data) != 3 {
			t.Fatalf("data len = %d, expected 3", len(resp.Data))
		}
		if got := resp.Data[0].TrainID; got != "T1030" {
			t.Fatalf("first train = %q, expected T1030", got)
		}
		if got := resp.Data[2].TrainID; got != "T1101" {
			t.Fatalf("last train = %q, expected T1101", got)
		}
		if got := resp.Data[0].DepartsAt.Format(time.RFC3339); got != "2026-03-08T10:30:00+07:00" {
			t.Fatalf("first departs_at = %q", got)
		}
	})

	t.Run("returns 400 for invalid departs_after", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "station_id", Value: "MRI"}}
		c.Request = httptest.NewRequest(
			http.MethodGet,
			"/v1/schedule/MRI?departs_after=bad-time",
			nil,
		)

		h.GetSchedules(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, expected 400", w.Code)
		}
	})

	t.Run("returns 400 for invalid filtered window_minutes", func(t *testing.T) {
		at := "2026-03-08T10:00:00+07:00"
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "station_id", Value: "MRI"}}
		c.Request = httptest.NewRequest(
			http.MethodGet,
			"/v1/schedule/MRI?window_minutes=121&departs_after="+url.QueryEscape(at),
			nil,
		)

		h.GetSchedules(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, expected 400", w.Code)
		}
	})
}
