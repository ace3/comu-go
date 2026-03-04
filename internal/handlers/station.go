package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/comuline/api/internal/cache"
	"github.com/comuline/api/internal/models"
	"github.com/comuline/api/internal/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StationHandler handles station-related endpoints.
type StationHandler struct {
	db    *gorm.DB
	cache *cache.Cache
}

// NewStationHandler creates a StationHandler.
func NewStationHandler(db *gorm.DB, c *cache.Cache) *StationHandler {
	return &StationHandler{db: db, cache: c}
}

// GetStations godoc
//
//	@Summary		List all stations
//	@Description	Returns all KRL stations, served from Redis cache when available.
//	@Tags			station
//	@Produce		json
//	@Param			page	query		int	false	"Page number (default 1)"
//	@Param			limit	query		int	false	"Results per page (default 100, max 500)"
//	@Success		200	{object}	response.PaginatedResponse{data=[]models.Station}
//	@Failure		500	{object}	response.Response
//	@Failure		503	{object}	response.Response
//	@Router			/v1/station [get]
func (h *StationHandler) GetStations(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	page, limit := parsePagination(c)
	cacheKey := paginationCacheKey("station:all", page, limit)

	var cached response.PaginatedResponse
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil {
		c.JSON(http.StatusOK, cached)
		return
	}

	var total int64
	if err := h.db.WithContext(ctx).Model(&models.Station{}).Count(&total).Error; err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to count stations")
		return
	}

	var stations []models.Station
	offset := (page - 1) * limit
	if err := h.db.WithContext(ctx).Offset(offset).Limit(limit).Find(&stations).Error; err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch stations")
		return
	}

	resp := response.BuildPaginatedSuccess(stations, page, limit, int(total))
	_ = h.cache.Set(ctx, cacheKey, resp, cache.TTLToMidnight())
	c.JSON(http.StatusOK, resp)
}

// GetStation godoc
//
//	@Summary		Get station by ID
//	@Description	Returns a single station by its uppercase ID (e.g. MRI for Manggarai).
//	@Tags			station
//	@Produce		json
//	@Param			id	path		string	true	"Station ID"
//	@Success		200	{object}	response.Response{data=models.Station}
//	@Failure		404	{object}	response.Response
//	@Failure		500	{object}	response.Response
//	@Failure		503	{object}	response.Response
//	@Router			/v1/station/{id} [get]
func (h *StationHandler) GetStation(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	id := strings.ToUpper(c.Param("id"))
	cacheKey := "station:" + id

	var station models.Station
	if err := h.cache.Get(ctx, cacheKey, &station); err == nil {
		response.BuildSuccess(c, station)
		return
	}

	if err := h.db.WithContext(ctx).Where("id = ?", id).First(&station).Error; err != nil {
		if ctx.Err() != nil {
			response.BuildError(c, http.StatusServiceUnavailable, "request timeout")
			return
		}
		response.BuildError(c, http.StatusNotFound, "station not found")
		return
	}

	_ = h.cache.Set(ctx, cacheKey, station, cache.TTLToMidnight())
	response.BuildSuccess(c, station)
}
