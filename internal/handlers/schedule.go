package handlers

import (
	"context"
	"net/http"
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
