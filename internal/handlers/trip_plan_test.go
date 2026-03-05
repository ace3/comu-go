package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
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

type rawScheduleRow struct {
	TrainID       string `json:"train_id"`
	Line          string `json:"line"`
	Route         string `json:"route"`
	OriginID      string `json:"origin_id"`
	DestinationID string `json:"destination_id"`
	StationID     string `json:"station_id"`
	DepartsAt     string `json:"departs_at"`
	ArrivesAt     string `json:"arrives_at"`
}

func mustWIBTime(t *testing.T, hhmm string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	ts, err := time.ParseInLocation("2006-01-02 15:04", "2026-03-05 "+hhmm, loc)
	if err != nil {
		t.Fatalf("parse time %s: %v", hhmm, err)
	}
	return ts
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

	t.Run("uses pass-through train even when destination label differs", func(t *testing.T) {
		db2 := setupTestDB(t)
		h2 := NewTripPlanHandler(db2, nil)
		rows := []models.Schedule{
			{ID: "x-sud", TrainID: "5555B", Line: "Commuter Line Cikarang", Route: "BKS-KPB", OriginID: "BKS", DestinationID: "KPB", StationID: "SUD", DepartsAt: mustWIBTime(t, "16:10"), ArrivesAt: mustWIBTime(t, "16:10")},
			{ID: "x-du", TrainID: "5555B", Line: "Commuter Line Cikarang", Route: "BKS-KPB", OriginID: "BKS", DestinationID: "KPB", StationID: "DU", DepartsAt: mustWIBTime(t, "16:26"), ArrivesAt: mustWIBTime(t, "16:26")},
			{ID: "y-du", TrainID: "1978A", Line: "Commuter Line Tangerang", Route: "DU-RW", OriginID: "DU", DestinationID: "RW", StationID: "DU", DepartsAt: mustWIBTime(t, "16:54"), ArrivesAt: mustWIBTime(t, "16:54")},
			{ID: "y-rw", TrainID: "1978A", Line: "Commuter Line Tangerang", Route: "DU-RW", OriginID: "DU", DestinationID: "RW", StationID: "RW", DepartsAt: mustWIBTime(t, "17:20"), ArrivesAt: mustWIBTime(t, "17:20")},
		}
		for _, row := range rows {
			if err := db2.Create(&row).Error; err != nil {
				t.Fatalf("seed row %s: %v", row.ID, err)
			}
		}

		body := map[string]any{
			"from_id":        "SUD",
			"to_id":          "RW",
			"at":             "2026-03-05T15:51:00+07:00",
			"window_minutes": 60,
			"max_results":    8,
			"max_transfers":  1,
		}
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/trip-plan", bytes.NewReader(raw))
		c.Request.Header.Set("Content-Type", "application/json")
		h2.GetTripPlan(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200, body=%s", w.Code, w.Body.String())
		}
		var resp tripPlanTestResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		if len(resp.Data.Options) == 0 {
			t.Fatalf("expected at least one option")
		}
		if len(resp.Data.Options[0].Legs) < 2 {
			t.Fatalf("expected transfer option")
		}
		if resp.Data.Options[0].Legs[0].TrainID != "5555B" {
			t.Fatalf("expected first leg train 5555B, got %s", resp.Data.Options[0].Legs[0].TrainID)
		}
	})

	t.Run("supports two transfers", func(t *testing.T) {
		db3 := setupTestDB(t)
		h3 := NewTripPlanHandler(db3, nil)
		rows := []models.Schedule{
			{ID: "a-a", TrainID: "T1", Line: "L1", Route: "A-B", OriginID: "A", DestinationID: "B", StationID: "A", DepartsAt: mustWIBTime(t, "16:00"), ArrivesAt: mustWIBTime(t, "16:00")},
			{ID: "a-b", TrainID: "T1", Line: "L1", Route: "A-B", OriginID: "A", DestinationID: "B", StationID: "B", DepartsAt: mustWIBTime(t, "16:10"), ArrivesAt: mustWIBTime(t, "16:10")},
			{ID: "b-b", TrainID: "T2", Line: "L2", Route: "B-C", OriginID: "B", DestinationID: "C", StationID: "B", DepartsAt: mustWIBTime(t, "16:14"), ArrivesAt: mustWIBTime(t, "16:14")},
			{ID: "b-c", TrainID: "T2", Line: "L2", Route: "B-C", OriginID: "B", DestinationID: "C", StationID: "C", DepartsAt: mustWIBTime(t, "16:24"), ArrivesAt: mustWIBTime(t, "16:24")},
			{ID: "c-c", TrainID: "T3", Line: "L3", Route: "C-D", OriginID: "C", DestinationID: "D", StationID: "C", DepartsAt: mustWIBTime(t, "16:28"), ArrivesAt: mustWIBTime(t, "16:28")},
			{ID: "c-d", TrainID: "T3", Line: "L3", Route: "C-D", OriginID: "C", DestinationID: "D", StationID: "D", DepartsAt: mustWIBTime(t, "16:40"), ArrivesAt: mustWIBTime(t, "16:40")},
		}
		for _, row := range rows {
			if err := db3.Create(&row).Error; err != nil {
				t.Fatalf("seed row %s: %v", row.ID, err)
			}
		}

		body := map[string]any{
			"from_id":        "A",
			"to_id":          "D",
			"at":             "2026-03-05T15:55:00+07:00",
			"window_minutes": 60,
			"max_results":    8,
			"max_transfers":  2,
		}
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/trip-plan", bytes.NewReader(raw))
		c.Request.Header.Set("Content-Type", "application/json")
		h3.GetTripPlan(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200, body=%s", w.Code, w.Body.String())
		}
		var resp tripPlanTestResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		if len(resp.Data.Options) == 0 {
			t.Fatalf("expected one option")
		}
		if len(resp.Data.Options[0].Legs) != 3 {
			t.Fatalf("expected 3 legs, got %d", len(resp.Data.Options[0].Legs))
		}
	})

	t.Run("uses cikarang via mri fallback stop at duri for transit", func(t *testing.T) {
		db4 := setupTestDB(t)
		h4 := NewTripPlanHandler(db4, nil)
		rows := []models.Schedule{
			{ID: "f-sud", TrainID: "5575C", Line: "Commuter Line Cikarang", Route: "CIKARANG-KAMPUNGBANDAN VIA MRI", OriginID: "CKR", DestinationID: "KPB", StationID: "SUD", DepartsAt: mustWIBTime(t, "20:20"), ArrivesAt: mustWIBTime(t, "20:20")},
			{ID: "f-thb", TrainID: "5575C", Line: "Commuter Line Cikarang", Route: "CIKARANG-KAMPUNGBANDAN VIA MRI", OriginID: "CKR", DestinationID: "KPB", StationID: "THB", DepartsAt: mustWIBTime(t, "20:29"), ArrivesAt: mustWIBTime(t, "20:29")},
			{ID: "f-ak", TrainID: "5575C", Line: "Commuter Line Cikarang", Route: "CIKARANG-KAMPUNGBANDAN VIA MRI", OriginID: "CKR", DestinationID: "KPB", StationID: "AK", DepartsAt: mustWIBTime(t, "20:40"), ArrivesAt: mustWIBTime(t, "20:40")},
			{ID: "f-kpb", TrainID: "5575C", Line: "Commuter Line Cikarang", Route: "CIKARANG-KAMPUNGBANDAN VIA MRI", OriginID: "CKR", DestinationID: "KPB", StationID: "KPB", DepartsAt: mustWIBTime(t, "20:47"), ArrivesAt: mustWIBTime(t, "20:47")},
			{ID: "g-du", TrainID: "1907A", Line: "Commuter Line Tangerang", Route: "DU-RW", OriginID: "DU", DestinationID: "RW", StationID: "DU", DepartsAt: mustWIBTime(t, "20:50"), ArrivesAt: mustWIBTime(t, "20:50")},
			{ID: "g-rw", TrainID: "1907A", Line: "Commuter Line Tangerang", Route: "DU-RW", OriginID: "DU", DestinationID: "RW", StationID: "RW", DepartsAt: mustWIBTime(t, "21:05"), ArrivesAt: mustWIBTime(t, "21:05")},
		}
		for _, row := range rows {
			if err := db4.Create(&row).Error; err != nil {
				t.Fatalf("seed row %s: %v", row.ID, err)
			}
		}

		body := map[string]any{
			"from_id":        "SUD",
			"to_id":          "RW",
			"at":             "2026-03-05T20:05:00+07:00",
			"window_minutes": 90,
			"max_results":    8,
			"max_transfers":  1,
		}
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/trip-plan", bytes.NewReader(raw))
		c.Request.Header.Set("Content-Type", "application/json")
		h4.GetTripPlan(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200, body=%s", w.Code, w.Body.String())
		}
		var resp tripPlanTestResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response: %v", err)
		}
		if len(resp.Data.Options) == 0 {
			t.Fatalf("expected at least one option")
		}

		foundFallbackTransit := false
		for _, opt := range resp.Data.Options {
			if len(opt.Legs) < 2 {
				continue
			}
			if opt.Legs[0].TrainID == "5575C" && opt.Legs[0].To == "DU" {
				foundFallbackTransit = true
				break
			}
		}
		if !foundFallbackTransit {
			t.Fatalf("expected option using 5575C with transit stop at DU")
		}
	})

	t.Run("marks projected metadata when using future at with snapshot reuse", func(t *testing.T) {
		body := map[string]any{
			"from_id":        "RW",
			"to_id":          "SUDB",
			"at":             "2026-03-08T15:51:00+07:00",
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
		if !resp.Metadata.Projected {
			t.Fatalf("expected metadata.projected=true")
		}
		if resp.Metadata.SnapshotDateWIB != "2026-03-05" {
			t.Fatalf("snapshot_date_wib = %q, expected 2026-03-05", resp.Metadata.SnapshotDateWIB)
		}
		if len(resp.Data.Options) == 0 {
			t.Fatalf("expected options > 0")
		}
	})
}

func TestAppendRouteFallbackStops_InsertsDuriBeforeAngke(t *testing.T) {
	route := []models.Schedule{
		{TrainID: "5575C", Line: "Commuter Line Cikarang", Route: "CIKARANG-KAMPUNGBANDAN VIA MRI", StationID: "SUD", DepartsAt: mustWIBTime(t, "20:20"), ArrivesAt: mustWIBTime(t, "20:20")},
		{TrainID: "5575C", Line: "Commuter Line Cikarang", Route: "CIKARANG-KAMPUNGBANDAN VIA MRI", StationID: "THB", DepartsAt: mustWIBTime(t, "20:29"), ArrivesAt: mustWIBTime(t, "20:29")},
		{TrainID: "5575C", Line: "Commuter Line Cikarang", Route: "CIKARANG-KAMPUNGBANDAN VIA MRI", StationID: "AK", DepartsAt: mustWIBTime(t, "20:40"), ArrivesAt: mustWIBTime(t, "20:40")},
		{TrainID: "5575C", Line: "Commuter Line Cikarang", Route: "CIKARANG-KAMPUNGBANDAN VIA MRI", StationID: "KPB", DepartsAt: mustWIBTime(t, "20:47"), ArrivesAt: mustWIBTime(t, "20:47")},
	}

	updated := appendRouteFallbackStops(route, "SUD", "CIKARANG-KAMPUNGBANDAN VIA MRI")
	duIdx := -1
	akIdx := -1
	for i, stop := range updated {
		switch stop.StationID {
		case "DU":
			duIdx = i
		case "AK":
			akIdx = i
		}
	}
	if duIdx < 0 {
		t.Fatalf("expected DU to be inserted")
	}
	if akIdx < 0 {
		t.Fatalf("expected AK to exist")
	}
	if duIdx >= akIdx {
		t.Fatalf("expected DU before AK, got duIdx=%d akIdx=%d", duIdx, akIdx)
	}
	if updated[duIdx].DepartsAt.Before(updated[duIdx-1].DepartsAt) || updated[duIdx].DepartsAt.After(updated[akIdx].DepartsAt) {
		t.Fatalf("expected inserted DU time between previous and AK stop")
	}
}

func TestAppendRouteFallbackStops_InsertsMRIAndPSEForViaRoutes(t *testing.T) {
	viaMRIRoute := []models.Schedule{
		{TrainID: "X1", Line: "Commuter Line Cikarang", Route: "CIKARANG-ANGKE VIA MRI", StationID: "KAT", DepartsAt: mustWIBTime(t, "20:10"), ArrivesAt: mustWIBTime(t, "20:10")},
		{TrainID: "X1", Line: "Commuter Line Cikarang", Route: "CIKARANG-ANGKE VIA MRI", StationID: "SUD", DepartsAt: mustWIBTime(t, "20:20"), ArrivesAt: mustWIBTime(t, "20:20")},
		{TrainID: "X1", Line: "Commuter Line Cikarang", Route: "CIKARANG-ANGKE VIA MRI", StationID: "AK", DepartsAt: mustWIBTime(t, "20:40"), ArrivesAt: mustWIBTime(t, "20:40")},
	}
	withMRI := appendRouteFallbackStops(viaMRIRoute, "KAT", "CIKARANG-ANGKE VIA MRI")
	hasMRI := false
	hasDU := false
	for _, stop := range withMRI {
		if stop.StationID == "MRI" {
			hasMRI = true
		}
		if stop.StationID == "DU" {
			hasDU = true
		}
	}
	if !hasMRI || !hasDU {
		t.Fatalf("expected VIA MRI route fallback to include MRI and DU, got MRI=%t DU=%t", hasMRI, hasDU)
	}

	viaPSERoute := []models.Schedule{
		{TrainID: "Y1", Line: "Commuter Line Cikarang", Route: "BEKASI-KAMPUNGBANDAN VIA PSE", StationID: "KMO", DepartsAt: mustWIBTime(t, "20:10"), ArrivesAt: mustWIBTime(t, "20:10")},
		{TrainID: "Y1", Line: "Commuter Line Cikarang", Route: "BEKASI-KAMPUNGBANDAN VIA PSE", StationID: "RJW", DepartsAt: mustWIBTime(t, "20:20"), ArrivesAt: mustWIBTime(t, "20:20")},
	}
	withPSE := appendRouteFallbackStops(viaPSERoute, "KMO", "BEKASI-KAMPUNGBANDAN VIA PSE")
	hasPSE := false
	for _, stop := range withPSE {
		if stop.StationID == "PSE" {
			hasPSE = true
			break
		}
	}
	if !hasPSE {
		t.Fatalf("expected VIA PSE route fallback to include PSE")
	}
}

func loadRealWindowRows(t *testing.T) []models.Schedule {
	t.Helper()
	schedulesPath := filepath.Join("..", "..", "data", "schedules.json")
	raw, err := os.ReadFile(schedulesPath)
	if err != nil {
		t.Fatalf("read schedules.json: %v", err)
	}

	var rows []rawScheduleRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("unmarshal schedules.json: %v", err)
	}

	layout := "2006-01-02 15:04:05-07"
	wib, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load wib location: %v", err)
	}
	startUTC := time.Date(2026, 3, 5, 1, 0, 0, 0, time.UTC) // 08:00 WIB
	endUTC := time.Date(2026, 3, 5, 3, 0, 0, 0, time.UTC)   // 10:00 WIB
	activeTrains := map[string]struct{}{}
	for _, row := range rows {
		dep, parseErr := time.Parse(layout, row.DepartsAt)
		if parseErr != nil {
			continue
		}
		if (dep.Equal(startUTC) || dep.After(startUTC)) && (dep.Equal(endUTC) || dep.Before(endUTC)) {
			activeTrains[row.TrainID] = struct{}{}
		}
	}

	out := make([]models.Schedule, 0, len(rows)/10)
	for _, row := range rows {
		if _, ok := activeTrains[row.TrainID]; !ok {
			continue
		}
		dep, depErr := time.Parse(layout, row.DepartsAt)
		arr, arrErr := time.Parse(layout, row.ArrivesAt)
		if depErr != nil || arrErr != nil {
			continue
		}
		out = append(out, models.Schedule{
			ID:            fmt.Sprintf("%s-%s-%s", row.TrainID, row.StationID, row.DepartsAt),
			TrainID:       row.TrainID,
			Line:          row.Line,
			Route:         row.Route,
			OriginID:      row.OriginID,
			DestinationID: row.DestinationID,
			StationID:     row.StationID,
			DepartsAt:     dep.In(wib),
			ArrivesAt:     arr.In(wib),
		})
	}
	return out
}

func TestTripPlanHandler_RealData_0800_1000WIB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	rows := loadRealWindowRows(t)
	if len(rows) == 0 {
		t.Fatalf("expected real-data rows for 08:00-10:00 WIB window")
	}
	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("seed real row %s: %v", row.ID, err)
		}
	}

	h := NewTripPlanHandler(db, nil)
	type pairCase struct {
		name string
		from string
		to   string
	}

	at := "2026-03-05T08:00:00+07:00"
	windowStart := mustWIBTime(t, "08:00")
	windowEnd := mustWIBTime(t, "10:00")
	byTrain := map[string][]models.Schedule{}
	for _, row := range rows {
		byTrain[row.TrainID] = append(byTrain[row.TrainID], row)
	}
	for trainID := range byTrain {
		sort.Slice(byTrain[trainID], func(i, j int) bool {
			return byTrain[trainID][i].DepartsAt.Before(byTrain[trainID][j].DepartsAt)
		})
	}

	seenPairs := map[string]struct{}{}
	cases := make([]pairCase, 0, 4)
	for trainID, route := range byTrain {
		if len(route) < 2 {
			continue
		}
		for i := 0; i < len(route)-1; i++ {
			depWIB := route[i].DepartsAt.In(windowStart.Location())
			if depWIB.Before(windowStart) || depWIB.After(windowEnd) {
				continue
			}
			from := route[i].StationID
			to := route[i+1].StationID
			if from == "" || to == "" || from == to {
				continue
			}
			key := from + "->" + to
			if _, ok := seenPairs[key]; ok {
				continue
			}
			seenPairs[key] = struct{}{}
			cases = append(cases, pairCase{
				name: fmt.Sprintf("%s_%s_to_%s", trainID, from, to),
				from: from,
				to:   to,
			})
			if len(cases) >= 4 {
				break
			}
		}
		if len(cases) >= 4 {
			break
		}
	}
	if len(cases) == 0 {
		t.Fatalf("expected at least one OD pair from 08:00-10:00 WIB real data")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{
				"from_id":        tc.from,
				"to_id":          tc.to,
				"at":             at,
				"window_minutes": 120,
				"max_results":    8,
				"max_transfers":  2,
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
				t.Fatalf("expected options for %s -> %s in real-data window", tc.from, tc.to)
			}
			for _, opt := range resp.Data.Options {
				if len(opt.Legs) > 3 {
					t.Fatalf("expected max 3 legs (<=2 transfers), got %d", len(opt.Legs))
				}
				if opt.DepartAt < at {
					t.Fatalf("option departAt %s earlier than test window start", opt.DepartAt)
				}
			}

			arrivals := make([]string, 0, len(resp.Data.Options))
			for _, opt := range resp.Data.Options {
				arrivals = append(arrivals, opt.ArriveAt)
			}
			sorted := append([]string(nil), arrivals...)
			sort.Strings(sorted)
			for i := range arrivals {
				if arrivals[i] != sorted[i] {
					t.Fatalf("options are not sorted by fastest arrival")
				}
			}
		})
	}
}
