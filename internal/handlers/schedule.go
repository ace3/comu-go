package handlers

import (
	"context"
	"net/http"

	"github.com/comuline/api/internal/cache"
	"github.com/comuline/api/internal/models"
	"github.com/comuline/api/internal/response"
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
//	@Success		200			{object}	response.Response{data=[]models.Schedule}
//	@Failure		500			{object}	response.Response
//	@Router			/v1/schedule/{station_id} [get]
func (h *ScheduleHandler) GetSchedules(c *gin.Context) {
	ctx := context.Background()
	stationID := c.Param("station_id")
	cacheKey := "schedule:" + stationID

	var schedules []models.Schedule
	if err := h.cache.Get(ctx, cacheKey, &schedules); err == nil {
		response.BuildSuccess(c, schedules)
		return
	}

	if err := h.db.Where("station_id = ?", stationID).Order("departs_at asc").Find(&schedules).Error; err != nil {
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch schedules")
		return
	}

	_ = h.cache.Set(ctx, cacheKey, schedules, cache.TTLToMidnight())
	response.BuildSuccess(c, schedules)
}
