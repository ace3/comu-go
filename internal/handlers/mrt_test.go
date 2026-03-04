package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/comuline/api/internal/models"
	"github.com/comuline/api/internal/response"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.Station{}, &models.Schedule{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func seedMRTStations(t *testing.T, db *gorm.DB) {
	t.Helper()
	stations := []models.Station{
		{UID: "lebak-bulus-grab", ID: "LBB", Name: "Lebak Bulus Grab", Type: "MRT", Metadata: datatypes.JSON(`{"nid":"20","order":1,"origin":{"color":"#DD0067"}}`)},
		{UID: "fatmawati-indomaret", ID: "FTM", Name: "Fatmawati Indomaret", Type: "MRT", Metadata: datatypes.JSON(`{"nid":"21","order":2,"origin":{"color":"#DD0067"}}`)},
		{UID: "bundaran-hi", ID: "BHI", Name: "Bundaran HI", Type: "MRT", Metadata: datatypes.JSON(`{"nid":"31","order":13,"origin":{"color":"#DD0067"}}`)},
	}
	for _, s := range stations {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("failed to seed station %s: %v", s.ID, err)
		}
	}
}

func seedMRTSchedules(t *testing.T, db *gorm.DB) {
	t.Helper()
	schedules := []models.Schedule{
		{
			ID: "LBB-MRT-HI-001", TrainID: "MRT-HI-001", Line: "MRT NORTH-SOUTH LINE",
			Route: "LEBAKBULUS-BUNDARANHI", OriginID: "LBB", DestinationID: "BHI",
			StationID: "LBB", Metadata: datatypes.JSON(`{"is_active":true,"origin":{"color":"#DD0067"}}`),
		},
		{
			ID: "LBB-MRT-HI-002", TrainID: "MRT-HI-002", Line: "MRT NORTH-SOUTH LINE",
			Route: "LEBAKBULUS-BUNDARANHI", OriginID: "LBB", DestinationID: "BHI",
			StationID: "LBB", Metadata: datatypes.JSON(`{"is_active":true,"origin":{"color":"#DD0067"}}`),
		},
	}
	for _, s := range schedules {
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("failed to seed schedule %s: %v", s.ID, err)
		}
	}
}

func seedKRLStation(t *testing.T, db *gorm.DB) {
	t.Helper()
	station := models.Station{
		UID: "manggarai", ID: "MRI", Name: "Manggarai", Type: "KRL",
		Metadata: datatypes.JSON(`{"daop":1,"fg_enable":1}`),
	}
	if err := db.Create(&station).Error; err != nil {
		t.Fatalf("failed to seed KRL station: %v", err)
	}
}

type testResponse struct {
	Metadata response.Metadata `json:"metadata"`
	Data     json.RawMessage   `json:"data"`
}

func TestMRTHandler_GetMRTStations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	seedMRTStations(t, db)
	seedKRLStation(t, db)

	handler := NewMRTHandler(db, nil)

	t.Run("returns only MRT stations", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/mrt/stations", nil)

		handler.GetMRTStations(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200", w.Code)
		}

		var resp testResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if !resp.Metadata.Success {
			t.Fatal("expected success=true")
		}

		var stations []models.Station
		if err := json.Unmarshal(resp.Data, &stations); err != nil {
			t.Fatalf("failed to parse data: %v", err)
		}

		if len(stations) != 3 {
			t.Errorf("expected 3 MRT stations, got %d", len(stations))
		}
		for _, s := range stations {
			if s.Type != "MRT" {
				t.Errorf("station %s has type %q, expected MRT", s.ID, s.Type)
			}
		}
	})
}

func TestMRTHandler_GetMRTStation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	seedMRTStations(t, db)
	seedKRLStation(t, db)

	handler := NewMRTHandler(db, nil)

	t.Run("returns MRT station by ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/mrt/stations/LBB", nil)
		c.Params = gin.Params{{Key: "id", Value: "LBB"}}

		handler.GetMRTStation(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200", w.Code)
		}

		var resp testResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		var station models.Station
		json.Unmarshal(resp.Data, &station)

		if station.ID != "LBB" {
			t.Errorf("station ID = %q, expected LBB", station.ID)
		}
		if station.Type != "MRT" {
			t.Errorf("station type = %q, expected MRT", station.Type)
		}
	})

	t.Run("returns 404 for KRL station", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/mrt/stations/MRI", nil)
		c.Params = gin.Params{{Key: "id", Value: "MRI"}}

		handler.GetMRTStation(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, expected 404", w.Code)
		}
	})

	t.Run("returns 404 for non-existent station", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/mrt/stations/XXX", nil)
		c.Params = gin.Params{{Key: "id", Value: "XXX"}}

		handler.GetMRTStation(c)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, expected 404", w.Code)
		}
	})
}

func TestMRTHandler_GetMRTSchedules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	seedMRTStations(t, db)
	seedMRTSchedules(t, db)

	handler := NewMRTHandler(db, nil)

	t.Run("returns MRT schedules for station", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/mrt/schedules/LBB", nil)
		c.Params = gin.Params{{Key: "station_id", Value: "LBB"}}

		handler.GetMRTSchedules(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200", w.Code)
		}

		var resp testResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		var schedules []models.Schedule
		json.Unmarshal(resp.Data, &schedules)

		if len(schedules) != 2 {
			t.Errorf("expected 2 schedules, got %d", len(schedules))
		}
		for _, s := range schedules {
			if s.Line != "MRT NORTH-SOUTH LINE" {
				t.Errorf("schedule line = %q, expected MRT NORTH-SOUTH LINE", s.Line)
			}
		}
	})

	t.Run("returns empty for station with no schedules", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/mrt/schedules/BHI", nil)
		c.Params = gin.Params{{Key: "station_id", Value: "BHI"}}

		handler.GetMRTSchedules(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200", w.Code)
		}

		var resp testResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		var schedules []models.Schedule
		json.Unmarshal(resp.Data, &schedules)

		if len(schedules) != 0 {
			t.Errorf("expected 0 schedules, got %d", len(schedules))
		}
	})
}

func TestMRTHandler_GetMRTRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	seedMRTStations(t, db)
	seedKRLStation(t, db)

	handler := NewMRTHandler(db, nil)

	t.Run("returns MRT route with stations", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/v1/mrt/routes", nil)

		handler.GetMRTRoutes(c)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, expected 200", w.Code)
		}

		var resp testResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		var routeResp MRTRouteResponse
		json.Unmarshal(resp.Data, &routeResp)

		if routeResp.Line != "MRT NORTH-SOUTH LINE" {
			t.Errorf("line = %q, expected MRT NORTH-SOUTH LINE", routeResp.Line)
		}
		if routeResp.Color != "#DD0067" {
			t.Errorf("color = %q, expected #DD0067", routeResp.Color)
		}
		if len(routeResp.Stops) != 3 {
			t.Errorf("expected 3 stops, got %d", len(routeResp.Stops))
		}
	})
}
