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
	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/models"
	"github.com/comu/api/internal/response"
	"github.com/comu/api/internal/scheduler"
	"github.com/comu/api/internal/scheduleview"
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

var routeFallbackStopsByKey = map[string][]string{
	"CIKARANGKAMPUNGBANDANVIAMRI": {"DU"},
}

var fallbackInsertBeforeCandidates = map[string][]string{
	"DU":  {"AK", "KPB"},
	"MRI": {"SUD", "SUDB", "THB", "DU", "AK", "KPB"},
	"PSE": {"RJW", "KPB"},
}

type TripPlanHandler struct {
	db    *gorm.DB
	cache *cache.Cache
	cfg   *config.Config
	view  *scheduleview.Service
}

func NewTripPlanHandler(db *gorm.DB, c *cache.Cache, cfg ...*config.Config) *TripPlanHandler {
	handlerCfg := &config.Config{
		OnDemandSyncEnabled:            false,
		OnDemandSyncMinIntervalMinutes: 30,
	}
	if len(cfg) > 0 && cfg[0] != nil {
		handlerCfg = cfg[0]
	}
	return &TripPlanHandler{db: db, cache: c, cfg: handlerCfg, view: scheduleview.New(db)}
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

	firstLegSchedules, projectionMeta, err := h.view.ProjectForTripPlan(ctx, []string{fromID}, now, windowEnd, 120)
	if err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch origin schedules")
		return
	}

	plannerMeta := response.Metadata{
		Success:         true,
		Projected:       projectionMeta.Projected,
		SnapshotDateWIB: projectionMeta.SnapshotDateWIB,
		SnapshotAgeDays: projectionMeta.SnapshotAgeDays,
	}

	if len(firstLegSchedules) == 0 {
		if plannerMeta.Projected || !projectionMeta.HasSnapshot {
			plannerMeta.SyncTriggered = scheduler.MaybeTriggerScheduleSync(h.cfg, h.db, h.cache, "trip_plan_no_first_leg")
		}
		response.BuildSuccessWithMetadata(c, plannerMeta, tripPlanResponse{
			Options: []tripPlanOption{},
			Stats:   tripPlanStats{ExpandedStates: 0, Truncated: false},
		})
		return
	}

	firstTrainIDs := uniqTrainIDs(firstLegSchedules)
	firstRoutes, routeMeta, err := h.view.ProjectRoutesByTrain(ctx, firstTrainIDs, now)
	if err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch routes")
		return
	}
	if routeMeta.Projected {
		plannerMeta.Projected = true
	}
	if plannerMeta.SnapshotDateWIB == "" {
		plannerMeta.SnapshotDateWIB = routeMeta.SnapshotDateWIB
	}
	if plannerMeta.SnapshotAgeDays < routeMeta.SnapshotAgeDays {
		plannerMeta.SnapshotAgeDays = routeMeta.SnapshotAgeDays
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
		route = appendRouteFallbackStops(route, fromID, first.Route)
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

			// ArrivesAt in the DB stores the DestTime (final destination arrival), not the
			// arrival at this specific stop. Use DepartsAt as the proxy for "time at this
			// station" — it avoids midnight-crossing bugs from overnight DestTime values.
			arriveAt := stop.DepartsAt

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
		if plannerMeta.Projected {
			plannerMeta.SyncTriggered = scheduler.MaybeTriggerScheduleSync(h.cfg, h.db, h.cache, "trip_plan_direct")
		}
		response.BuildSuccessWithMetadata(c, plannerMeta, tripPlanResponse{
			Options: direct,
			Stats:   tripPlanStats{ExpandedStates: expanded, Truncated: false},
		})
		return
	}

	if len(seedsByTransfer) == 0 || maxTransfers == 0 {
		if plannerMeta.Projected {
			plannerMeta.SyncTriggered = scheduler.MaybeTriggerScheduleSync(h.cfg, h.db, h.cache, "trip_plan_no_transfer_paths")
		}
		response.BuildSuccessWithMetadata(c, plannerMeta, tripPlanResponse{
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

		departures, depMeta, err := h.view.ProjectForTripPlan(ctx, transferIDs, earliestTransferArrival.Add(tripMinTransfer), latestTransferArrival.Add(tripMaxTransfer), 0)
		if err != nil {
			if ctx.Err() != nil {
				response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
				return
			}
			response.BuildError(c, http.StatusInternalServerError, "failed to fetch transfer schedules")
			return
		}
		if depMeta.Projected {
			plannerMeta.Projected = true
		}
		if plannerMeta.SnapshotDateWIB == "" {
			plannerMeta.SnapshotDateWIB = depMeta.SnapshotDateWIB
		}
		if plannerMeta.SnapshotAgeDays < depMeta.SnapshotAgeDays {
			plannerMeta.SnapshotAgeDays = depMeta.SnapshotAgeDays
		}

		routesByTrain, transferRouteMeta, err := h.view.ProjectRoutesByTrain(ctx, uniqTrainIDs(departures), now)
		if err != nil {
			if ctx.Err() != nil {
				response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
				return
			}
			response.BuildError(c, http.StatusInternalServerError, "failed to fetch transfer routes")
			return
		}
		if transferRouteMeta.Projected {
			plannerMeta.Projected = true
		}
		if plannerMeta.SnapshotDateWIB == "" {
			plannerMeta.SnapshotDateWIB = transferRouteMeta.SnapshotDateWIB
		}
		if plannerMeta.SnapshotAgeDays < transferRouteMeta.SnapshotAgeDays {
			plannerMeta.SnapshotAgeDays = transferRouteMeta.SnapshotAgeDays
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
				route = appendRouteFallbackStops(route, transferID, dep.Route)
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
						// ArrivesAt is the DestTime (final destination), not this stop's arrival.
						stopArrive := stop.DepartsAt
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
	if plannerMeta.Projected || !projectionMeta.HasSnapshot {
		plannerMeta.SyncTriggered = scheduler.MaybeTriggerScheduleSync(h.cfg, h.db, h.cache, "trip_plan_final")
	}
	response.BuildSuccessWithMetadata(c, plannerMeta, tripPlanResponse{
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
		return appendViaFallbackStops(route, routeName)
	}
	for _, stop := range route {
		if strings.ToUpper(stop.StationID) == terminalID {
			return appendViaFallbackStops(route, routeName)
		}
	}
	if len(route) == 0 {
		return route
	}
	last := route[len(route)-1]
	// ArrivesAt stores DestTime from the KRL API = arrival time at the final destination.
	// Use it directly if it's after the last departure (it already IS the terminal time).
	// Fall back to DepartsAt+5min only when ArrivesAt is not meaningful.
	tail := last.DepartsAt.Add(5 * time.Minute)
	if !last.ArrivesAt.IsZero() && last.ArrivesAt.After(last.DepartsAt) {
		tail = last.ArrivesAt
	}
	fallback := models.Schedule{
		TrainID:   last.TrainID,
		Line:      last.Line,
		Route:     routeName,
		StationID: terminalID,
		DepartsAt: tail,
		ArrivesAt: tail,
	}
	withTerminal := append(route, fallback)
	return appendViaFallbackStops(withTerminal, routeName)
}

func appendRouteFallbackStops(route []models.Schedule, fromID, routeName string) []models.Schedule {
	return appendTerminalFallbackStops(route, fromID, routeName)
}

func normalizeRouteKey(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func appendViaFallbackStops(route []models.Schedule, routeName string) []models.Schedule {
	key := normalizeRouteKey(routeName)
	if key == "" {
		return route
	}
	extraStops := append([]string(nil), routeFallbackStopsByKey[key]...)
	if strings.Contains(key, "VIAMRI") {
		extraStops = append(extraStops, "DU", "MRI")
	}
	if strings.Contains(key, "VIAPSE") {
		extraStops = append(extraStops, "PSE")
	}
	extraStops = uniqStationIDs(extraStops)
	if len(extraStops) == 0 {
		return route
	}

	withFallbacks := route
	for _, stationID := range extraStops {
		withFallbacks = upsertFallbackStop(withFallbacks, stationID, routeName)
	}
	return withFallbacks
}

func uniqStationIDs(stations []string) []string {
	if len(stations) == 0 {
		return nil
	}
	out := make([]string, 0, len(stations))
	seen := map[string]struct{}{}
	for _, stationID := range stations {
		id := strings.ToUpper(strings.TrimSpace(stationID))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func upsertFallbackStop(route []models.Schedule, stationID, routeName string) []models.Schedule {
	if len(route) == 0 || strings.TrimSpace(stationID) == "" {
		return route
	}
	stationID = strings.ToUpper(strings.TrimSpace(stationID))
	for _, stop := range route {
		if strings.ToUpper(stop.StationID) == stationID {
			return route
		}
	}

	insertBefore := -1
	beforeCandidates := fallbackInsertBeforeCandidates[stationID]
	if len(beforeCandidates) == 0 {
		beforeCandidates = []string{"AK", "KPB"}
	}
	for _, candidate := range beforeCandidates {
		for i, stop := range route {
			if strings.ToUpper(stop.StationID) == candidate {
				insertBefore = i
				break
			}
		}
		if insertBefore >= 0 {
			break
		}
	}

	newStop := models.Schedule{
		TrainID: route[len(route)-1].TrainID,
		Line:    route[len(route)-1].Line,
		Route:   routeName,
	}
	newStop.StationID = stationID

	if insertBefore <= 0 {
		tail := route[len(route)-1].ArrivesAt
		if tail.IsZero() {
			tail = route[len(route)-1].DepartsAt
		}
		newStop.DepartsAt = tail.Add(5 * time.Minute)
		newStop.ArrivesAt = newStop.DepartsAt
		return append(route, newStop)
	}

	prev := route[insertBefore-1]
	next := route[insertBefore]
	prevT := prev.ArrivesAt
	if prevT.IsZero() {
		prevT = prev.DepartsAt
	}
	nextT := next.DepartsAt
	if nextT.IsZero() {
		nextT = next.ArrivesAt
	}
	mid := prevT.Add(nextT.Sub(prevT) / 2)
	if !nextT.After(prevT) {
		mid = nextT.Add(-1 * time.Minute)
	}
	newStop.DepartsAt = mid
	newStop.ArrivesAt = mid

	updated := make([]models.Schedule, 0, len(route)+1)
	updated = append(updated, route[:insertBefore]...)
	updated = append(updated, newStop)
	updated = append(updated, route[insertBefore:]...)
	return updated
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
