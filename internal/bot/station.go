// Package bot provides the Telegram bot core logic for KRL Commuter.
package bot

import (
	"strings"
	"unicode"

	"github.com/comu/api/internal/models"
)

// FindStation performs a fuzzy/case-insensitive search for KRL stations by name or code.
// It returns all stations whose name or code contains the query (case-insensitive).
func FindStation(stations []models.Station, query string) []models.Station {
	q := normalize(query)
	var exact []models.Station
	var results []models.Station
	for _, s := range stations {
		if s.Type != "KRL" {
			continue
		}
		name := normalize(s.Name)
		if name == q || strings.EqualFold(s.ID, q) {
			exact = append(exact, s)
			continue
		}
		if strings.Contains(name, q) {
			results = append(results, s)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return results
}

// normalize lowercases and strips extra spaces from a string.
func normalize(s string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(s)))
}
