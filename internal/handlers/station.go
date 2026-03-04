package handlers

import (
	"context"
	"net/http"
	"strings"

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
//	@Success		200	{object}	response.Response{data=[]models.Station}
//	@Failure		500	{object}	response.Response
//	@Router			/v1/station [get]
func (h *StationHandler) GetStations(c *gin.Context) {
	ctx := context.Background()
	const cacheKey = "station:all"

	var stations []models.Station
	if err := h.cache.Get(ctx, cacheKey, &stations); err == nil {
		response.BuildSuccess(c, stations)
		return
	}

	if err := h.db.Find(&stations).Error; err != nil {
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch stations")
		return
	}

	_ = h.cache.Set(ctx, cacheKey, stations, cache.TTLToMidnight())
	response.BuildSuccess(c, stations)
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
//	@Router			/v1/station/{id} [get]
func (h *StationHandler) GetStation(c *gin.Context) {
	ctx := context.Background()
	id := strings.ToUpper(c.Param("id"))
	cacheKey := "station:" + id

	var station models.Station
	if err := h.cache.Get(ctx, cacheKey, &station); err == nil {
		response.BuildSuccess(c, station)
		return
	}

	if err := h.db.Where("id = ?", id).First(&station).Error; err != nil {
		response.BuildError(c, http.StatusNotFound, "station not found")
		return
	}

	_ = h.cache.Set(ctx, cacheKey, station, cache.TTLToMidnight())
	response.BuildSuccess(c, station)
}
