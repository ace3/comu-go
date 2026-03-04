package sync

import (
	"strings"
	"testing"
)

func TestBuildMRTStations(t *testing.T) {
	stations := BuildMRTStations()

	t.Run("returns correct number of stations", func(t *testing.T) {
		if len(stations) != 13 {
			t.Errorf("expected 13 stations, got %d", len(stations))
		}
	})

	t.Run("all stations have MRT type", func(t *testing.T) {
		for _, s := range stations {
			if s.Type != "MRT" {
				t.Errorf("station %s has type %q, expected MRT", s.ID, s.Type)
			}
		}
	})

	t.Run("all stations have unique IDs", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, s := range stations {
			if seen[s.ID] {
				t.Errorf("duplicate station ID: %s", s.ID)
			}
			seen[s.ID] = true
		}
	})

	t.Run("all stations have unique UIDs", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, s := range stations {
			if seen[s.UID] {
				t.Errorf("duplicate station UID: %s", s.UID)
			}
			seen[s.UID] = true
		}
	})

	t.Run("all stations have non-empty fields", func(t *testing.T) {
		for _, s := range stations {
			if s.UID == "" {
				t.Error("station has empty UID")
			}
			if s.ID == "" {
				t.Error("station has empty ID")
			}
			if s.Name == "" {
				t.Error("station has empty Name")
			}
			if len(s.Metadata) == 0 {
				t.Errorf("station %s has empty Metadata", s.ID)
			}
		}
	})

	t.Run("first station is Lebak Bulus", func(t *testing.T) {
		if stations[0].ID != "LBB" {
			t.Errorf("first station ID = %q, expected LBB", stations[0].ID)
		}
		if stations[0].Name != "Lebak Bulus Grab" {
			t.Errorf("first station name = %q, expected 'Lebak Bulus Grab'", stations[0].Name)
		}
	})

	t.Run("last station is Bundaran HI", func(t *testing.T) {
		last := stations[len(stations)-1]
		if last.ID != "BHI" {
			t.Errorf("last station ID = %q, expected BHI", last.ID)
		}
		if last.Name != "Bundaran HI" {
			t.Errorf("last station name = %q, expected 'Bundaran HI'", last.Name)
		}
	})

	t.Run("metadata contains origin color", func(t *testing.T) {
		for _, s := range stations {
			metadata := string(s.Metadata)
			if metadata == "" {
				t.Errorf("station %s has empty metadata", s.ID)
				continue
			}
			if !strings.Contains(metadata, "#DD0067") {
				t.Errorf("station %s metadata missing MRT color #DD0067: %s", s.ID, metadata)
			}
		}
	})
}

func TestMRTStationDefinitions(t *testing.T) {
	t.Run("station order is sequential", func(t *testing.T) {
		for i, def := range mrtStationDefinitions {
			if def.Order != i+1 {
				t.Errorf("station %s has order %d, expected %d", def.ID, def.Order, i+1)
			}
		}
	})
}
