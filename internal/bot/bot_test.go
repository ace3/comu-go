package bot

import (
	"testing"
	"time"

	"github.com/comu/api/internal/models"
	"gorm.io/datatypes"
)

func TestFindStation(t *testing.T) {
	stations := []models.Station{
		{UID: "depok", ID: "DP", Name: "Depok", Type: "KRL", Metadata: datatypes.JSON(`{}`)},
		{UID: "depok-baru", ID: "DPB", Name: "Depok Baru", Type: "KRL", Metadata: datatypes.JSON(`{}`)},
		{UID: "depok-timur", ID: "DPT", Name: "Depok Timur", Type: "KRL", Metadata: datatypes.JSON(`{}`)},
		{UID: "jakarta-kota", ID: "JAKK", Name: "Jakarta Kota", Type: "KRL", Metadata: datatypes.JSON(`{}`)},
		{UID: "bogor", ID: "BGR", Name: "Bogor", Type: "KRL", Metadata: datatypes.JSON(`{}`)},
		{UID: "lebak-bulus", ID: "LBB", Name: "Lebak Bulus", Type: "MRT", Metadata: datatypes.JSON(`{}`)}, // should be excluded
	}

	tests := []struct {
		name     string
		query    string
		wantLen  int
		wantCode []string
	}{
		{"exact name match", "Depok", 3, []string{"DP", "DPB", "DPT"}},
		{"partial name match", "jakarta", 1, []string{"JAKK"}},
		{"case insensitive", "depok", 3, nil},
		{"code match", "BGR", 1, []string{"BGR"}},
		{"no match", "xyz123", 0, nil},
		{"MRT excluded", "Lebak", 0, nil},
		{"bogor", "bogo", 1, []string{"BGR"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := FindStation(stations, tt.query)
			if len(results) != tt.wantLen {
				t.Errorf("FindStation(%q) returned %d results, want %d", tt.query, len(results), tt.wantLen)
			}
			if tt.wantCode != nil {
				codes := make(map[string]bool)
				for _, r := range results {
					codes[r.ID] = true
				}
				for _, c := range tt.wantCode {
					if !codes[c] {
						t.Errorf("expected station %s in results, not found", c)
					}
				}
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Jakarta Kota", "jakarta kota"},
		{"  Depok  ", "depok"},
		{"BOGOR", "bogor"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalize(tt.input)
			if got != tt.want {
				t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidTime(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"07:00", true},
		{"17:30", true},
		{"00:00", true},
		{"23:59", true},
		{"7:00", true}, // Go's time.Parse("15:04") accepts single-digit hours
		{"25:00", false},
		{"abc", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidTime(tt.input)
			if got != tt.valid {
				t.Errorf("isValidTime(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestParseWorkDays(t *testing.T) {
	tests := []struct {
		input   string
		wantLen int
		wantDays []string
	}{
		{"mon,tue,wed,thu,fri", 5, []string{"mon", "tue", "wed", "thu", "fri"}},
		{"mon, tue, wed", 3, nil},
		{"sat,sun", 2, []string{"sat", "sun"}},
		{"invalid,mon", 1, []string{"mon"}},
		{"", 0, nil},
		{"MON,TUE", 2, []string{"mon", "tue"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseWorkDays(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("parseWorkDays(%q) returned %d days, want %d (got %v)", tt.input, len(got), tt.wantLen, got)
			}
			if tt.wantDays != nil {
				daySet := make(map[string]bool)
				for _, d := range got {
					daySet[d] = true
				}
				for _, d := range tt.wantDays {
					if !daySet[d] {
						t.Errorf("expected day %s in result, not found in %v", d, got)
					}
				}
			}
		})
	}
}

func TestParseTimeInput_Now(t *testing.T) {
	loc, _ := loadJakartaLoc()
	result, err := parseTimeInput("now", loc)
	if err != nil {
		t.Fatalf("parseTimeInput(now) error: %v", err)
	}
	if result.IsZero() {
		t.Error("expected non-zero time for 'now'")
	}
}

func TestParseTimeInput_Tomorrow(t *testing.T) {
	loc, _ := loadJakartaLoc()
	result, err := parseTimeInput("tomorrow 08:00", loc)
	if err != nil {
		t.Fatalf("parseTimeInput(tomorrow 08:00) error: %v", err)
	}
	if result.Hour() != 8 || result.Minute() != 0 {
		t.Errorf("expected 08:00, got %s", result.Format("15:04"))
	}
}

func TestParseTimeInput_Today(t *testing.T) {
	loc, _ := loadJakartaLoc()
	result, err := parseTimeInput("today 15:30", loc)
	if err != nil {
		t.Fatalf("parseTimeInput(today 15:30) error: %v", err)
	}
	if result.Hour() != 15 || result.Minute() != 30 {
		t.Errorf("expected 15:30, got %s", result.Format("15:04"))
	}
}

func TestParseTimeInput_Invalid(t *testing.T) {
	loc, _ := loadJakartaLoc()
	_, err := parseTimeInput("not-a-time", loc)
	if err == nil {
		t.Error("expected error for invalid time input")
	}
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Jakarta Kota", "Jakarta Kota"},
		{"Jakarta_Kota", "Jakarta\\_Kota"},
		{"*bold*", "\\*bold\\*"},
		{"[link]", "\\[link]"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("escapeMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func loadJakartaLoc() (*time.Location, error) {
	return time.LoadLocation("Asia/Jakarta")
}
