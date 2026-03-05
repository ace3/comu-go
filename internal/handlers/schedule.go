package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/comu/api/internal/cache"
	"github.com/comu/api/internal/models"
	"github.com/comu/api/internal/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ScheduleHandler handles schedule-related endpoints.
type ScheduleHandler struct {
	db    *gorm.DB
	cache *cache.Cache
}

// NewScheduleHandler creates a ScheduleHandler.
func NewScheduleHandler(db *gorm.DB, c *cache.Cache) *ScheduleHandler {
	return &ScheduleHandler{db: db, cache: c}
}

// GetSchedules godoc
//
//	@Summary		Get schedules by station ID
//	@Description	Returns all train schedules for a station, ordered by departure time.
//	@Tags			schedule
//	@Produce		json
//	@Param			station_id	path		string	true	"Station ID"
//	@Param			page		query		int		false	"Page number (default 1)"
//	@Param			limit		query		int		false	"Results per page (default 100, max 500)"
//	@Success		200			{object}	response.PaginatedResponse{data=[]models.Schedule}
//	@Failure		500			{object}	response.Response
//	@Failure		503			{object}	response.Response
//	@Router			/v1/schedule/{station_id} [get]
func (h *ScheduleHandler) GetSchedules(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	stationID := c.Param("station_id")
	page, limit := parsePagination(c)
	cacheKey := paginationCacheKey("schedule:"+stationID, page, limit)

	var cached response.PaginatedResponse
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil {
		c.JSON(http.StatusOK, cached)
		return
	}

	var total int64
	if err := h.db.WithContext(ctx).Model(&models.Schedule{}).Where("station_id = ?", stationID).Count(&total).Error; err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to count schedules")
		return
	}

	var schedules []models.Schedule
	offset := (page - 1) * limit
	if err := h.db.WithContext(ctx).Where("station_id = ?", stationID).Order("departs_at asc").Offset(offset).Limit(limit).Find(&schedules).Error; err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch schedules")
		return
	}

	resp := response.BuildPaginatedSuccess(schedules, page, limit, int(total))
	_ = h.cache.Set(ctx, cacheKey, resp, cache.TTLToMidnight())
	c.JSON(http.StatusOK, resp)
}

type scheduleWindowItem struct {
	TrainID       string    `json:"train_id"`
	Line          string    `json:"line"`
	Route         string    `json:"route"`
	OriginID      string    `json:"origin_id"`
	DestinationID string    `json:"destination_id"`
	StationID     string    `json:"station_id"`
	DepartsAt     time.Time `json:"departs_at"`
	ArrivesAt     time.Time `json:"arrives_at"`
}

type scheduleWindowStation struct {
	StationID string               `json:"station_id"`
	Schedules []scheduleWindowItem `json:"schedules"`
}

type scheduleWindowResponse struct {
	AtWIB          string                  `json:"at_wib"`
	WindowStartWIB string                  `json:"window_start_wib"`
	WindowEndWIB   string                  `json:"window_end_wib"`
	Stations       []scheduleWindowStation `json:"stations"`
}

// GetScheduleWindow godoc
//
//	@Summary		Get schedules for selected stations within a WIB time window
//	@Description	Returns schedules grouped by station for the specified stations in a ±window around `at` (Asia/Jakarta).
//	@Tags			schedule
//	@Produce		json
//	@Param			station_ids		query		string	true	"Comma-separated station IDs (max 5)"
//	@Param			window_minutes	query		int		false	"Window size in minutes (default 60, max 180)"
//	@Param			at				query		string	false	"Reference time in RFC3339 format (default now)"
//	@Success		200				{object}	response.Response{data=scheduleWindowResponse}
//	@Failure		400				{object}	response.Response
//	@Failure		500				{object}	response.Response
//	@Failure		503				{object}	response.Response
//	@Router			/v1/schedule/window [get]
func (h *ScheduleHandler) GetScheduleWindow(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	stationIDs, err := parseStationIDs(c.Query("station_ids"))
	if err != nil {
		response.BuildError(c, http.StatusBadRequest, err.Error())
		return
	}

	windowMinutes := 60
	if rawWindow := c.Query("window_minutes"); rawWindow != "" {
		value, convErr := strconv.Atoi(rawWindow)
		if convErr != nil || value < 1 || value > 180 {
			response.BuildError(c, http.StatusBadRequest, "window_minutes must be between 1 and 180")
			return
		}
		windowMinutes = value
	}

	wib, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		response.BuildError(c, http.StatusInternalServerError, "failed to load timezone")
		return
	}

	atWIB := time.Now().In(wib)
	if rawAt := c.Query("at"); rawAt != "" {
		parsedAt, parseErr := time.Parse(time.RFC3339, rawAt)
		if parseErr != nil {
			response.BuildError(c, http.StatusBadRequest, "at must be a valid RFC3339 timestamp")
			return
		}
		atWIB = parsedAt.In(wib)
	}

	window := time.Duration(windowMinutes) * time.Minute
	startWIB := atWIB.Add(-window)
	endWIB := atWIB.Add(window)

	var items []scheduleWindowItem
	query := h.db.WithContext(ctx).
		Model(&models.Schedule{}).
		Select("train_id", "line", "route", "origin_id", "destination_id", "station_id", "departs_at", "arrives_at").
		Where("station_id IN ? AND departs_at BETWEEN ? AND ?", stationIDs, startWIB, endWIB).
		Order("station_id asc, departs_at asc")
	if err := query.Find(&items).Error; err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch schedules")
		return
	}

	byStation := make(map[string][]scheduleWindowItem, len(stationIDs))
	for _, item := range items {
		byStation[item.StationID] = append(byStation[item.StationID], item)
	}

	stations := make([]scheduleWindowStation, 0, len(stationIDs))
	for _, stationID := range stationIDs {
		stations = append(stations, scheduleWindowStation{
			StationID: stationID,
			Schedules: byStation[stationID],
		})
	}

	response.BuildSuccess(c, scheduleWindowResponse{
		AtWIB:          atWIB.Format(time.RFC3339),
		WindowStartWIB: startWIB.Format(time.RFC3339),
		WindowEndWIB:   endWIB.Format(time.RFC3339),
		Stations:       stations,
	})
}

func parseStationIDs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errBadRequest("station_ids is required")
	}

	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		id := strings.ToUpper(strings.TrimSpace(p))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, errBadRequest("station_ids is required")
	}
	if len(ids) > 5 {
		return nil, errBadRequest("station_ids must contain at most 5 station IDs")
	}

	return ids, nil
}

type errBadRequest string

func (e errBadRequest) Error() string { return string(e) }
