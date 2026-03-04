package sync

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildMRTSchedules(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	// Use a Wednesday for weekday test.
	weekday := time.Date(2025, 3, 5, 12, 0, 0, 0, loc)
	// Use a Saturday for weekend test.
	weekend := time.Date(2025, 3, 8, 12, 0, 0, 0, loc)

	inputs := []MRTScheduleInput{
		{
			StationID: "LBB",
			Directions: []MRTScheduleDirection{
				{
					Location: "hi",
					Times: MRTTimes{
						Weekdays: []string{"05:00", "05:10", "05:20"},
						Weekends: []string{"06:00", "06:20"},
					},
				},
			},
		},
		{
			StationID: "BHI",
			Directions: []MRTScheduleDirection{
				{
					Location: "lb",
					Times: MRTTimes{
						Weekdays: []string{"05:30", "05:40"},
						Weekends: []string{"06:30"},
					},
				},
			},
		},
	}

	t.Run("weekday schedules", func(t *testing.T) {
		schedules := BuildMRTSchedules(inputs, weekday, loc)

		// LBB has 3 weekday northbound + BHI has 2 weekday southbound = 5
		if len(schedules) != 5 {
			t.Errorf("expected 5 schedules, got %d", len(schedules))
		}

		// Check first schedule
		first := schedules[0]
		if first.StationID != "LBB" {
			t.Errorf("first schedule station = %q, expected LBB", first.StationID)
		}
		if first.Line != mrtLine {
			t.Errorf("line = %q, expected %q", first.Line, mrtLine)
		}
		if first.Route != mrtRouteUp {
			t.Errorf("route = %q, expected %q", first.Route, mrtRouteUp)
		}
		if first.OriginID != "LBB" {
			t.Errorf("origin_id = %q, expected LBB", first.OriginID)
		}
		if first.DestinationID != "BHI" {
			t.Errorf("destination_id = %q, expected BHI", first.DestinationID)
		}
		if first.DepartsAt.Hour() != 5 || first.DepartsAt.Minute() != 0 {
			t.Errorf("departs_at = %v, expected 05:00", first.DepartsAt)
		}

		// Check southbound schedule
		lastStation := schedules[3]
		if lastStation.StationID != "BHI" {
			t.Errorf("schedule station = %q, expected BHI", lastStation.StationID)
		}
		if lastStation.Route != mrtRouteDown {
			t.Errorf("route = %q, expected %q", lastStation.Route, mrtRouteDown)
		}
		if lastStation.OriginID != "BHI" {
			t.Errorf("origin_id = %q, expected BHI", lastStation.OriginID)
		}
		if lastStation.DestinationID != "LBB" {
			t.Errorf("destination_id = %q, expected LBB", lastStation.DestinationID)
		}
	})

	t.Run("weekend schedules", func(t *testing.T) {
		schedules := BuildMRTSchedules(inputs, weekend, loc)

		// LBB has 2 weekend northbound + BHI has 1 weekend southbound = 3
		if len(schedules) != 3 {
			t.Errorf("expected 3 schedules, got %d", len(schedules))
		}

		first := schedules[0]
		if first.DepartsAt.Hour() != 6 || first.DepartsAt.Minute() != 0 {
			t.Errorf("weekend departs_at = %v, expected 06:00", first.DepartsAt)
		}
	})

	t.Run("unique schedule IDs", func(t *testing.T) {
		schedules := BuildMRTSchedules(inputs, weekday, loc)
		seen := make(map[string]bool)
		for _, s := range schedules {
			if seen[s.ID] {
				t.Errorf("duplicate schedule ID: %s", s.ID)
			}
			seen[s.ID] = true
		}
	})

	t.Run("metadata contains is_active and origin color", func(t *testing.T) {
		schedules := BuildMRTSchedules(inputs, weekday, loc)
		for _, s := range schedules {
			var meta map[string]any
			if err := json.Unmarshal(s.Metadata, &meta); err != nil {
				t.Errorf("failed to unmarshal metadata for %s: %v", s.ID, err)
				continue
			}
			if _, ok := meta["is_active"]; !ok {
				t.Errorf("schedule %s metadata missing is_active", s.ID)
			}
			origin, ok := meta["origin"].(map[string]any)
			if !ok {
				t.Errorf("schedule %s metadata missing origin", s.ID)
				continue
			}
			if color, ok := origin["color"].(string); !ok || color != "#DD0067" {
				t.Errorf("schedule %s origin color = %v, expected #DD0067", s.ID, origin["color"])
			}
		}
	})

	t.Run("train IDs encode direction", func(t *testing.T) {
		schedules := BuildMRTSchedules(inputs, weekday, loc)
		for _, s := range schedules {
			if !strings.HasPrefix(s.TrainID, "MRT-") {
				t.Errorf("train ID %q doesn't start with MRT-", s.TrainID)
			}
			if s.Route == mrtRouteUp && !strings.Contains(s.TrainID, "-HI-") {
				t.Errorf("northbound train ID %q should contain -HI-", s.TrainID)
			}
			if s.Route == mrtRouteDown && !strings.Contains(s.TrainID, "-LB-") {
				t.Errorf("southbound train ID %q should contain -LB-", s.TrainID)
			}
		}
	})
}

func TestBuildMRTScheduleInputs(t *testing.T) {
	inputs := buildMRTScheduleInputs()

	t.Run("covers all 13 stations", func(t *testing.T) {
		if len(inputs) != 13 {
			t.Errorf("expected 13 station inputs, got %d", len(inputs))
		}
	})

	t.Run("LBB only has northbound", func(t *testing.T) {
		lbb := inputs[0]
		if lbb.StationID != "LBB" {
			t.Fatalf("first station = %q, expected LBB", lbb.StationID)
		}
		if len(lbb.Directions) != 1 {
			t.Errorf("LBB has %d directions, expected 1", len(lbb.Directions))
		}
		if lbb.Directions[0].Location != "hi" {
			t.Errorf("LBB direction = %q, expected hi", lbb.Directions[0].Location)
		}
	})

	t.Run("BHI only has southbound", func(t *testing.T) {
		bhi := inputs[len(inputs)-1]
		if bhi.StationID != "BHI" {
			t.Fatalf("last station = %q, expected BHI", bhi.StationID)
		}
		if len(bhi.Directions) != 1 {
			t.Errorf("BHI has %d directions, expected 1", len(bhi.Directions))
		}
		if bhi.Directions[0].Location != "lb" {
			t.Errorf("BHI direction = %q, expected lb", bhi.Directions[0].Location)
		}
	})

	t.Run("intermediate stations have both directions", func(t *testing.T) {
		for _, input := range inputs[1 : len(inputs)-1] {
			if len(input.Directions) != 2 {
				t.Errorf("station %s has %d directions, expected 2", input.StationID, len(input.Directions))
			}
		}
	})

	t.Run("all times are valid HH:MM format", func(t *testing.T) {
		for _, input := range inputs {
			for _, dir := range input.Directions {
				for _, timeStr := range dir.Times.Weekdays {
					if !isValidTimeFormat(timeStr) {
						t.Errorf("station %s (%s) has invalid weekday time: %q", input.StationID, dir.Location, timeStr)
					}
				}
				for _, timeStr := range dir.Times.Weekends {
					if !isValidTimeFormat(timeStr) {
						t.Errorf("station %s (%s) has invalid weekend time: %q", input.StationID, dir.Location, timeStr)
					}
				}
			}
		}
	})

	t.Run("weekday and weekend schedules are non-empty", func(t *testing.T) {
		for _, input := range inputs {
			for _, dir := range input.Directions {
				if len(dir.Times.Weekdays) == 0 {
					t.Errorf("station %s (%s) has empty weekday schedule", input.StationID, dir.Location)
				}
				if len(dir.Times.Weekends) == 0 {
					t.Errorf("station %s (%s) has empty weekend schedule", input.StationID, dir.Location)
				}
			}
		}
	})

	t.Run("times are in ascending order", func(t *testing.T) {
		for _, input := range inputs {
			for _, dir := range input.Directions {
				for i := 1; i < len(dir.Times.Weekdays); i++ {
					if dir.Times.Weekdays[i] <= dir.Times.Weekdays[i-1] {
						t.Errorf("station %s (%s) weekday times not ascending at index %d: %s <= %s",
							input.StationID, dir.Location, i, dir.Times.Weekdays[i], dir.Times.Weekdays[i-1])
					}
				}
				for i := 1; i < len(dir.Times.Weekends); i++ {
					if dir.Times.Weekends[i] <= dir.Times.Weekends[i-1] {
						t.Errorf("station %s (%s) weekend times not ascending at index %d: %s <= %s",
							input.StationID, dir.Location, i, dir.Times.Weekends[i], dir.Times.Weekends[i-1])
					}
				}
			}
		}
	})
}

func isValidTimeFormat(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return false
	}
	if len(parts[0]) != 2 || len(parts[1]) != 2 {
		return false
	}
	h := 0
	m := 0
	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			return false
		}
		h = h*10 + int(c-'0')
	}
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
		m = m*10 + int(c-'0')
	}
	return h >= 0 && h <= 23 && m >= 0 && m <= 59
}
