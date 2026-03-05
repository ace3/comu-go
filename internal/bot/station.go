// Package bot provides the Telegram bot core logic for KRL Commuter.
package bot

import (
	"strings"
	"unicode"

	"github.com/comuline/api/internal/models"
)

// FindStation performs a fuzzy/case-insensitive search for KRL stations by name or code.
// It returns all stations whose name or code contains the query (case-insensitive).
func FindStation(stations []models.Station, query string) []models.Station {
	q := normalize(query)
	var results []models.Station
	for _, s := range stations {
		if s.Type != "KRL" {
			continue
		}
		if strings.Contains(normalize(s.Name), q) || strings.EqualFold(s.ID, q) {
			results = append(results, s)
		}
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
