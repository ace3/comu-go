package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/comu/api/internal/cache"
	"github.com/comu/api/internal/models"
	"github.com/comu/api/internal/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	tripMinTransfer = 2 * time.Minute
	tripMaxTransfer = 240 * time.Minute
)

var terminalCodeByName = map[string]string{
	"TANGERANG":           "TNG",
	"DURI":                "DU",
	"MANGGARAI":           "MRI",
	"TANAHABANG":          "THB",
	"SUDIRMANBARU":        "SUDB",
	"SUDIRMAN":            "SUD",
	"KARET":               "KAT",
	"BOGOR":               "BOO",
	"JAKARTAKOTA":         "JAKK",
	"KAMPUNGBANDAN":       "KPB",
	"CIKARANG":            "CKR",
	"BEKASI":              "BKS",
	"ANGKE":               "AK",
	"TAMBUN":              "TB",
	"BANDARSOEKARNOHATTA": "BST",
	"SOEKARNOHATTA":       "BST",
	"RANGKASBITUNG":       "RK",
	"MERAK":               "MRK",
}

type TripPlanHandler struct {
	db    *gorm.DB
	cache *cache.Cache
}

func NewTripPlanHandler(db *gorm.DB, c *cache.Cache) *TripPlanHandler {
	return &TripPlanHandler{db: db, cache: c}
}

type tripPlanRequest struct {
	FromID        string `json:"from_id" binding:"required"`
	ToID          string `json:"to_id" binding:"required"`
	At            string `json:"at,omitempty"`
	WindowMinutes int    `json:"window_minutes,omitempty"`
	MaxResults    int    `json:"max_results,omitempty"`
	MaxTransfers  int    `json:"max_transfers,omitempty"`
}

type tripPlanLeg struct {
	TrainID  string    `json:"trainId"`
	Line     string    `json:"line"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	DepartAt time.Time `json:"departAt"`
	ArriveAt time.Time `json:"arriveAt"`
}

type tripPlanOption struct {
	Legs            []tripPlanLeg `json:"legs"`
	DepartAt        time.Time     `json:"departAt"`
	ArriveAt        time.Time     `json:"arriveAt"`
	DurationMinutes int           `json:"durationMinutes"`
}

type tripPlanStats struct {
	ExpandedStates int  `json:"expanded_states"`
	Truncated      bool `json:"truncated"`
}

type tripPlanResponse struct {
	Options []tripPlanOption `json:"options"`
	Stats   tripPlanStats    `json:"stats"`
}

type tripSeed struct {
	legs      []tripPlanLeg
	arriveAt  time.Time
	stationID string
}

// GetTripPlan godoc
//
//	@Summary		Get trip plan options
//	@Description	Returns direct and one-transfer trip options for from/to within a future time window.
//	@Tags			planner
//	@Accept			json
//	@Produce		json
//	@Param			request	body		tripPlanRequest	true	"Trip planning request"
//	@Success		200		{object}	response.Response{data=tripPlanResponse}
//	@Failure		400		{object}	response.Response
//	@Failure		500		{object}	response.Response
//	@Failure		503		{object}	response.Response
//	@Router			/v1/trip-plan [post]
func (h *TripPlanHandler) GetTripPlan(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()

	var req tripPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BuildError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	fromID := strings.ToUpper(strings.TrimSpace(req.FromID))
	toID := strings.ToUpper(strings.TrimSpace(req.ToID))
	if fromID == "" || toID == "" || fromID == toID {
		response.BuildError(c, http.StatusBadRequest, "from_id and to_id are required and must be different")
		return
	}

	windowMinutes := req.WindowMinutes
	if windowMinutes == 0 {
		windowMinutes = 60
	}
	if windowMinutes < 1 || windowMinutes > 180 {
		response.BuildError(c, http.StatusBadRequest, "window_minutes must be between 1 and 180")
		return
	}

	maxResults := req.MaxResults
	if maxResults == 0 {
		maxResults = 8
	}
	if maxResults < 1 || maxResults > 30 {
		response.BuildError(c, http.StatusBadRequest, "max_results must be between 1 and 30")
		return
	}
	maxTransfers := req.MaxTransfers
	if maxTransfers == 0 {
		maxTransfers = 2
	}
	if maxTransfers < 0 || maxTransfers > 2 {
		response.BuildError(c, http.StatusBadRequest, "max_transfers must be between 0 and 2")
		return
	}

	wib, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		response.BuildError(c, http.StatusInternalServerError, "failed to load timezone")
		return
	}

	now := time.Now().In(wib)
	if strings.TrimSpace(req.At) != "" {
		at, parseErr := time.Parse(time.RFC3339, req.At)
		if parseErr != nil {
			response.BuildError(c, http.StatusBadRequest, "at must be RFC3339")
			return
		}
		now = at.In(wib)
	}
	windowEnd := now.Add(time.Duration(windowMinutes) * time.Minute)

	var firstLegSchedules []models.Schedule
	if err := h.db.WithContext(ctx).
		Where("station_id = ? AND departs_at BETWEEN ? AND ?", fromID, now, windowEnd).
		Order("departs_at asc").
		Limit(120).
		Find(&firstLegSchedules).Error; err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch origin schedules")
		return
	}

	if len(firstLegSchedules) == 0 {
		response.BuildSuccess(c, tripPlanResponse{
			Options: []tripPlanOption{},
			Stats:   tripPlanStats{ExpandedStates: 0, Truncated: false},
		})
		return
	}

	firstTrainIDs := uniqTrainIDs(firstLegSchedules)
	firstRoutes, err := loadRoutesByTrain(ctx, h.db, firstTrainIDs)
	if err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch routes")
		return
	}

	options := make([]tripPlanOption, 0, maxResults*16)
	seen := make(map[string]struct{}, maxResults*16)
	seedsByTransfer := map[string][]tripSeed{}
	expanded := 0

	for _, first := range firstLegSchedules {
		route := firstRoutes[first.TrainID]
		if len(route) == 0 {
			continue
		}
		route = appendTerminalFallbackStops(route, fromID, first.Route)
		fromIdx := findRouteIndex(route, fromID, first.DepartsAt.Add(-tripMinTransfer))
		if fromIdx < 0 {
			continue
		}
		maxForward := fromIdx + 30
		if maxForward > len(route) {
			maxForward = len(route)
		}
		for i := fromIdx + 1; i < maxForward; i++ {
			stop := route[i]
			stopStation := strings.ToUpper(stop.StationID)
			if stopStation == "" || stopStation == fromID {
				continue
			}
			expanded++

			arriveAt := stop.ArrivesAt
			if arriveAt.IsZero() {
				arriveAt = stop.DepartsAt
			}

			seed := tripSeed{
				legs: []tripPlanLeg{{
					TrainID:  first.TrainID,
					Line:     first.Line,
					From:     fromID,
					To:       stopStation,
					DepartAt: first.DepartsAt,
					ArriveAt: arriveAt,
				}},
				arriveAt:  arriveAt,
				stationID: stopStation,
			}

			if stopStation == toID {
				pushOption(&options, seen, tripPlanOption{
					Legs:            append([]tripPlanLeg{}, seed.legs...),
					DepartAt:        seed.legs[0].DepartAt,
					ArriveAt:        seed.arriveAt,
					DurationMinutes: minutesDiff(seed.legs[0].DepartAt, seed.arriveAt),
				})
				continue
			}

			bucket := seedsByTransfer[seed.stationID]
			if len(bucket) < 8 {
				seedsByTransfer[seed.stationID] = append(bucket, seed)
			}
		}
	}

	if direct := finalizeTripOptions(options, maxResults); len(direct) > 0 && len(direct[0].Legs) == 1 {
		response.BuildSuccess(c, tripPlanResponse{
			Options: direct,
			Stats:   tripPlanStats{ExpandedStates: expanded, Truncated: false},
		})
		return
	}

	if len(seedsByTransfer) == 0 || maxTransfers == 0 {
		response.BuildSuccess(c, tripPlanResponse{
			Options: []tripPlanOption{},
			Stats:   tripPlanStats{ExpandedStates: expanded, Truncated: false},
		})
		return
	}

	currentSeeds := seedsByTransfer
	for transferStep := 1; transferStep <= maxTransfers; transferStep++ {
		if len(currentSeeds) == 0 {
			break
		}
		transferIDs := make([]string, 0, len(currentSeeds))
		earliestTransferArrival := time.Time{}
		latestTransferArrival := time.Time{}
		for transferID, seeds := range currentSeeds {
			transferIDs = append(transferIDs, transferID)
			for _, s := range seeds {
				if earliestTransferArrival.IsZero() || s.arriveAt.Before(earliestTransferArrival) {
					earliestTransferArrival = s.arriveAt
				}
				if latestTransferArrival.IsZero() || s.arriveAt.After(latestTransferArrival) {
					latestTransferArrival = s.arriveAt
				}
			}
		}
		sort.Strings(transferIDs)

		var departures []models.Schedule
		if err := h.db.WithContext(ctx).
			Where("station_id IN ? AND departs_at BETWEEN ? AND ?", transferIDs, earliestTransferArrival.Add(tripMinTransfer), latestTransferArrival.Add(tripMaxTransfer)).
			Order("station_id asc, departs_at asc").
			Find(&departures).Error; err != nil {
			if ctx.Err() != nil {
				response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
				return
			}
			response.BuildError(c, http.StatusInternalServerError, "failed to fetch transfer schedules")
			return
		}

		routesByTrain, err := loadRoutesByTrain(ctx, h.db, uniqTrainIDs(departures))
		if err != nil {
			if ctx.Err() != nil {
				response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
				return
			}
			response.BuildError(c, http.StatusInternalServerError, "failed to fetch transfer routes")
			return
		}

		byTransfer := map[string][]models.Schedule{}
		for _, s := range departures {
			stationID := strings.ToUpper(s.StationID)
			byTransfer[stationID] = append(byTransfer[stationID], s)
		}

		nextSeeds := map[string][]tripSeed{}
		nextSeedSeen := map[string]struct{}{}
		for _, transferID := range transferIDs {
			seedPaths := currentSeeds[transferID]
			deps := byTransfer[transferID]
			if len(seedPaths) == 0 || len(deps) == 0 {
				continue
			}
			if len(deps) > 120 {
				deps = deps[:120]
			}

			for _, dep := range deps {
				expanded++
				route := routesByTrain[dep.TrainID]
				if len(route) == 0 {
					continue
				}
				route = appendTerminalFallbackStops(route, transferID, dep.Route)
				fromIdx := findRouteIndex(route, transferID, dep.DepartsAt.Add(-tripMinTransfer))
				if fromIdx < 0 {
					continue
				}
				maxForward := fromIdx + 30
				if maxForward > len(route) {
					maxForward = len(route)
				}

				for _, seed := range seedPaths {
					gap := dep.DepartsAt.Sub(seed.arriveAt)
					if gap < tripMinTransfer || gap > tripMaxTransfer {
						continue
					}

					for i := fromIdx + 1; i < maxForward; i++ {
						stop := route[i]
						stopStation := strings.ToUpper(stop.StationID)
						if stopStation == "" || stopStation == transferID {
							continue
						}
						stopArrive := stop.ArrivesAt
						if stopArrive.IsZero() {
							stopArrive = stop.DepartsAt
						}
						leg := tripPlanLeg{
							TrainID:  dep.TrainID,
							Line:     dep.Line,
							From:     transferID,
							To:       stopStation,
							DepartAt: dep.DepartsAt,
							ArriveAt: stopArrive,
						}
						newLegs := append(append([]tripPlanLeg{}, seed.legs...), leg)
						if stopStation == toID {
							pushOption(&options, seen, tripPlanOption{
								Legs:            newLegs,
								DepartAt:        newLegs[0].DepartAt,
								ArriveAt:        stopArrive,
								DurationMinutes: minutesDiff(newLegs[0].DepartAt, stopArrive),
							})
							continue
						}

						if transferStep >= maxTransfers {
							continue
						}
						next := tripSeed{
							legs:      newLegs,
							arriveAt:  stopArrive,
							stationID: stopStation,
						}
						key := optionKey(tripPlanOption{Legs: next.legs, DepartAt: next.legs[0].DepartAt, ArriveAt: next.arriveAt})
						if _, ok := nextSeedSeen[key]; ok {
							continue
						}
						nextSeedSeen[key] = struct{}{}
						b := nextSeeds[stopStation]
						if len(b) < 8 {
							nextSeeds[stopStation] = append(b, next)
						}
					}
				}
			}
		}
		currentSeeds = nextSeeds
	}

	final := finalizeTripOptions(options, maxResults)
	response.BuildSuccess(c, tripPlanResponse{
		Options: final,
		Stats:   tripPlanStats{ExpandedStates: expanded, Truncated: false},
	})
}

func uniqTrainIDs(schedules []models.Schedule) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(schedules))
	for _, s := range schedules {
		id := strings.ToUpper(strings.TrimSpace(s.TrainID))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func loadRoutesByTrain(ctx context.Context, db *gorm.DB, trainIDs []string) (map[string][]models.Schedule, error) {
	out := make(map[string][]models.Schedule, len(trainIDs))
	if len(trainIDs) == 0 {
		return out, nil
	}
	var rows []models.Schedule
	if err := db.WithContext(ctx).Where("train_id IN ?", trainIDs).Order("train_id asc, departs_at asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		id := strings.ToUpper(strings.TrimSpace(row.TrainID))
		out[id] = append(out[id], row)
	}
	return out, nil
}

func findRouteIndex(route []models.Schedule, stationID string, minTime time.Time) int {
	for i, stop := range route {
		if strings.ToUpper(stop.StationID) != stationID {
			continue
		}
		if minTime.IsZero() || !stop.DepartsAt.Before(minTime) {
			return i
		}
	}
	for i, stop := range route {
		if strings.ToUpper(stop.StationID) == stationID {
			return i
		}
	}
	return -1
}

func findDestinationForward(route []models.Schedule, fromIdx int, toID string) int {
	if fromIdx < 0 || fromIdx >= len(route) {
		return -1
	}
	for i := fromIdx + 1; i < len(route); i++ {
		if strings.ToUpper(route[i].StationID) == toID {
			return i
		}
	}
	return -1
}

func normalizeStationToken(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func parseRouteTerminalID(routeName string) string {
	route := strings.TrimSpace(routeName)
	if !strings.Contains(route, "-") {
		return ""
	}
	parts := strings.Split(route, "-")
	if len(parts) < 2 {
		return ""
	}
	token := normalizeStationToken(parts[len(parts)-1])
	if token == "" {
		return ""
	}
	return terminalCodeByName[token]
}

func appendTerminalFallbackStops(route []models.Schedule, fromID, routeName string) []models.Schedule {
	terminalID := parseRouteTerminalID(routeName)
	if terminalID == "" || terminalID == fromID {
		return route
	}
	for _, stop := range route {
		if strings.ToUpper(stop.StationID) == terminalID {
			return route
		}
	}
	if len(route) == 0 {
		return route
	}
	last := route[len(route)-1]
	tail := last.ArrivesAt
	if tail.IsZero() {
		tail = last.DepartsAt
	}
	fallback := models.Schedule{
		TrainID:   last.TrainID,
		Line:      last.Line,
		Route:     routeName,
		StationID: terminalID,
		DepartsAt: tail.Add(5 * time.Minute),
		ArrivesAt: tail.Add(5 * time.Minute),
	}
	return append(route, fallback)
}

func pushOption(options *[]tripPlanOption, seen map[string]struct{}, option tripPlanOption) {
	key := optionKey(option)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*options = append(*options, option)
}

func optionKey(option tripPlanOption) string {
	parts := make([]string, 0, len(option.Legs))
	for _, leg := range option.Legs {
		parts = append(parts, fmt.Sprintf("%s:%s->%s@%s", leg.TrainID, leg.From, leg.To, leg.DepartAt.Format(time.RFC3339)))
	}
	return strings.Join(parts, "|")
}

func minutesDiff(from, to time.Time) int {
	d := to.Sub(from)
	if d < 0 {
		return 0
	}
	return int(d.Round(time.Minute) / time.Minute)
}

func transferWaitMinutes(option tripPlanOption) int {
	if len(option.Legs) < 2 {
		return 0
	}
	return minutesDiff(option.Legs[0].ArriveAt, option.Legs[1].DepartAt)
}

func transferWaitRank(option tripPlanOption) int {
	if len(option.Legs) < 2 {
		return 0
	}
	wait := transferWaitMinutes(option)
	if wait < 5 {
		return 2
	}
	if wait > 25 {
		return 1
	}
	return 0
}

func transferTargetDelta(option tripPlanOption) int {
	if len(option.Legs) < 2 {
		return 0
	}
	wait := transferWaitMinutes(option)
	if wait >= 10 {
		return wait - 10
	}
	return 10 - wait
}

func isDominated(candidate, challenger tripPlanOption) bool {
	if len(candidate.Legs) < 2 || len(candidate.Legs) != len(challenger.Legs) {
		return false
	}
	departsNoEarlier := !challenger.DepartAt.Before(candidate.DepartAt)
	arrivesNoLater := !challenger.ArriveAt.After(candidate.ArriveAt)
	strictlyBetter := challenger.DepartAt.After(candidate.DepartAt) || challenger.ArriveAt.Before(candidate.ArriveAt)
	return departsNoEarlier && arrivesNoLater && strictlyBetter
}

func filterDominated(options []tripPlanOption) []tripPlanOption {
	if len(options) < 2 {
		return options
	}
	out := make([]tripPlanOption, 0, len(options))
	for i := range options {
		dominated := false
		for j := range options {
			if i == j {
				continue
			}
			if isDominated(options[i], options[j]) {
				dominated = true
				break
			}
		}
		if !dominated {
			out = append(out, options[i])
		}
	}
	return out
}

func finalizeTripOptions(options []tripPlanOption, maxResults int) []tripPlanOption {
	if len(options) == 0 {
		return []tripPlanOption{}
	}
	optimal := filterDominated(options)
	sort.SliceStable(optimal, func(i, j int) bool {
		a := optimal[i]
		b := optimal[j]
		if len(a.Legs) != len(b.Legs) {
			return len(a.Legs) < len(b.Legs)
		}
		if !a.ArriveAt.Equal(b.ArriveAt) {
			return a.ArriveAt.Before(b.ArriveAt)
		}
		if transferWaitRank(a) != transferWaitRank(b) {
			return transferWaitRank(a) < transferWaitRank(b)
		}
		if transferTargetDelta(a) != transferTargetDelta(b) {
			return transferTargetDelta(a) < transferTargetDelta(b)
		}
		if transferWaitMinutes(a) != transferWaitMinutes(b) {
			return transferWaitMinutes(a) < transferWaitMinutes(b)
		}
		if !a.DepartAt.Equal(b.DepartAt) {
			return a.DepartAt.After(b.DepartAt)
		}
		return a.DurationMinutes < b.DurationMinutes
	})

	direct := make([]tripPlanOption, 0, len(optimal))
	for _, o := range optimal {
		if len(o.Legs) == 1 {
			direct = append(direct, o)
		}
	}
	if len(direct) > 0 {
		if len(direct) > maxResults {
			return direct[:maxResults]
		}
		return direct
	}

	minLegs := len(optimal[0].Legs)
	filtered := make([]tripPlanOption, 0, len(optimal))
	for _, o := range optimal {
		if len(o.Legs) == minLegs {
			filtered = append(filtered, o)
		}
	}
	if len(filtered) > maxResults {
		return filtered[:maxResults]
	}
	return filtered
}
