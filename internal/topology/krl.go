package topology

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type KRLTopology struct {
	Corridors    map[string][]string `json:"corridors"`
	Interchanges map[string][]string `json:"interchanges"`

	indexByCorridor map[string]map[string]int
}

var (
	loadOnce sync.Once
	cached   *KRLTopology
	loadErr  error
)

func LoadKRLTopology() (*KRLTopology, error) {
	loadOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		baseDir := filepath.Join(filepath.Dir(file), "..", "..", "web", "krl_topology.json")
		raw, err := os.ReadFile(baseDir)
		if err != nil {
			loadErr = err
			return
		}
		var graph KRLTopology
		if err := json.Unmarshal(raw, &graph); err != nil {
			loadErr = err
			return
		}
		graph.indexByCorridor = make(map[string]map[string]int, len(graph.Corridors))
		for corridorID, stations := range graph.Corridors {
			idx := make(map[string]int, len(stations))
			for i, stationID := range stations {
				idx[strings.ToUpper(strings.TrimSpace(stationID))] = i
			}
			graph.indexByCorridor[corridorID] = idx
		}
		cached = &graph
	})
	return cached, loadErr
}

func (t *KRLTopology) IsInterchange(stationID string) bool {
	if t == nil {
		return false
	}
	_, ok := t.Interchanges[strings.ToUpper(strings.TrimSpace(stationID))]
	return ok
}

func (t *KRLTopology) CanReach(corridorID, fromID, toID string) bool {
	if t == nil {
		return false
	}
	idx := t.indexByCorridor[corridorID]
	if idx == nil {
		return false
	}
	_, fromOK := idx[strings.ToUpper(strings.TrimSpace(fromID))]
	_, toOK := idx[strings.ToUpper(strings.TrimSpace(toID))]
	return fromOK && toOK
}

func (t *KRLTopology) ClassifyRoute(routeName string, stops []string) (string, bool) {
	if t == nil {
		return "", false
	}
	normalizedRoute := normalizeRoute(routeName)
	for _, hint := range []struct {
		token      string
		corridorID string
	}{
		{"VIAMRI", "cikarang_via_mri"},
		{"VIAPSE", "cikarang_via_pse"},
		{"TANGERANG", "tangerang"},
		{"NAMBO", "bogor_nambo"},
		{"MERAK", "merak"},
		{"RANGKASBITUNG", "rangkasbitung"},
		{"TANJUNGPRIOK", "tanjung_priok"},
		{"SOEKARNOHATTA", "airport"},
		{"BANDARA", "airport"},
	} {
		if strings.Contains(normalizedRoute, hint.token) {
			return hint.corridorID, true
		}
	}

	candidates := make([]string, 0, 2)
	for corridorID, idx := range t.indexByCorridor {
		matches := true
		for _, stop := range stops {
			if stop == "" {
				continue
			}
			if _, ok := idx[strings.ToUpper(strings.TrimSpace(stop))]; !ok {
				matches = false
				break
			}
		}
		if matches {
			candidates = append(candidates, corridorID)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return "", false
}

func normalizeRoute(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
