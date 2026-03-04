package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/comuline/api/internal/cache"
	"github.com/comuline/api/internal/models"
	"github.com/comuline/api/internal/response"
	syncer "github.com/comuline/api/internal/sync"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// MRTHandler handles MRT-specific endpoints.
type MRTHandler struct {
	db    *gorm.DB
	cache *cache.Cache
}

// NewMRTHandler creates a MRTHandler.
func NewMRTHandler(db *gorm.DB, c *cache.Cache) *MRTHandler {
	return &MRTHandler{db: db, cache: c}
}

// GetMRTStations godoc
//
//	@Summary		List all MRT stations
//	@Description	Returns all MRT Jakarta North-South Line stations.
//	@Tags			mrt
//	@Produce		json
//	@Success		200	{object}	response.Response{data=[]models.Station}
//	@Failure		500	{object}	response.Response
//	@Router			/v1/mrt/stations [get]
func (h *MRTHandler) GetMRTStations(c *gin.Context) {
	ctx := context.Background()
	const cacheKey = "mrt:station:all"

	var stations []models.Station
	if h.cache != nil {
		if err := h.cache.Get(ctx, cacheKey, &stations); err == nil {
			response.BuildSuccess(c, stations)
			return
		}
	}

	if err := h.db.Where("type = ?", "MRT").Order("uid asc").Find(&stations).Error; err != nil {
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch MRT stations")
		return
	}

	if h.cache != nil {
		_ = h.cache.Set(ctx, cacheKey, stations, cache.TTLToMidnight())
	}
	response.BuildSuccess(c, stations)
}

// GetMRTStation godoc
//
//	@Summary		Get MRT station by ID
//	@Description	Returns a single MRT station by its uppercase ID (e.g. LBB for Lebak Bulus).
//	@Tags			mrt
//	@Produce		json
//	@Param			id	path		string	true	"Station ID"
//	@Success		200	{object}	response.Response{data=models.Station}
//	@Failure		404	{object}	response.Response
//	@Failure		500	{object}	response.Response
//	@Router			/v1/mrt/stations/{id} [get]
func (h *MRTHandler) GetMRTStation(c *gin.Context) {
	ctx := context.Background()
	id := strings.ToUpper(c.Param("id"))
	cacheKey := "mrt:station:" + id

	var station models.Station
	if h.cache != nil {
		if err := h.cache.Get(ctx, cacheKey, &station); err == nil {
			response.BuildSuccess(c, station)
			return
		}
	}

	if err := h.db.Where("id = ? AND type = ?", id, "MRT").First(&station).Error; err != nil {
		response.BuildError(c, http.StatusNotFound, "MRT station not found")
		return
	}

	if h.cache != nil {
		_ = h.cache.Set(ctx, cacheKey, station, cache.TTLToMidnight())
	}
	response.BuildSuccess(c, station)
}

// GetMRTSchedules godoc
//
//	@Summary		Get MRT schedules by station ID
//	@Description	Returns all MRT train schedules for a station, ordered by departure time.
//	@Tags			mrt
//	@Produce		json
//	@Param			station_id	path		string	true	"Station ID"
//	@Success		200			{object}	response.Response{data=[]models.Schedule}
//	@Failure		500			{object}	response.Response
//	@Router			/v1/mrt/schedules/{station_id} [get]
func (h *MRTHandler) GetMRTSchedules(c *gin.Context) {
	ctx := context.Background()
	stationID := strings.ToUpper(c.Param("station_id"))
	cacheKey := "mrt:schedule:" + stationID

	var schedules []models.Schedule
	if h.cache != nil {
		if err := h.cache.Get(ctx, cacheKey, &schedules); err == nil {
			response.BuildSuccess(c, schedules)
			return
		}
	}

	if err := h.db.Where("station_id = ? AND line = ?", stationID, "MRT NORTH-SOUTH LINE").
		Order("departs_at asc").Find(&schedules).Error; err != nil {
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch MRT schedules")
		return
	}

	if h.cache != nil {
		_ = h.cache.Set(ctx, cacheKey, schedules, cache.TTLToMidnight())
	}
	response.BuildSuccess(c, schedules)
}

// GetMRTRoutes godoc
//
//	@Summary		Get MRT route information
//	@Description	Returns the MRT North-South Line route with all stations in order.
//	@Tags			mrt
//	@Produce		json
//	@Success		200	{object}	response.Response{data=MRTRouteResponse}
//	@Failure		500	{object}	response.Response
//	@Router			/v1/mrt/routes [get]
func (h *MRTHandler) GetMRTRoutes(c *gin.Context) {
	ctx := context.Background()
	const cacheKey = "mrt:routes"

	var routeResp MRTRouteResponse
	if h.cache != nil {
		if err := h.cache.Get(ctx, cacheKey, &routeResp); err == nil {
			response.BuildSuccess(c, routeResp)
			return
		}
	}

	// Use the hardcoded station definitions to ensure correct ordering.
	mrtStations := syncer.BuildMRTStations()
	stops := make([]MRTRouteStop, len(mrtStations))
	for i, s := range mrtStations {
		stops[i] = MRTRouteStop{
			StationID: s.ID,
			Name:      s.Name,
		}
	}

	routeResp = MRTRouteResponse{
		Line:   "MRT NORTH-SOUTH LINE",
		Color:  "#DD0067",
		Stops:  stops,
	}

	if h.cache != nil {
		_ = h.cache.Set(ctx, cacheKey, routeResp, cache.TTLToMidnight())
	}
	response.BuildSuccess(c, routeResp)
}

// MRTRouteStop represents a stop on the MRT route.
type MRTRouteStop struct {
	StationID string `json:"station_id"`
	Name      string `json:"name"`
}

// MRTRouteResponse is the response for MRT route endpoints.
type MRTRouteResponse struct {
	Line  string         `json:"line"`
	Color string         `json:"color"`
	Stops []MRTRouteStop `json:"stops"`
}
