package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/comuline/api/internal/cache"
	"github.com/comuline/api/internal/models"
	"github.com/comuline/api/internal/response"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RouteHandler handles train route endpoints.
type RouteHandler struct {
	db    *gorm.DB
	cache *cache.Cache
}

// NewRouteHandler creates a RouteHandler.
func NewRouteHandler(db *gorm.DB, c *cache.Cache) *RouteHandler {
	return &RouteHandler{db: db, cache: c}
}

// RouteStop represents one stop in a train's route.
type RouteStop struct {
	StationID string    `json:"station_id"`
	DepartsAt time.Time `json:"departs_at"`
	ArrivesAt time.Time `json:"arrives_at"`
}

// RouteDetails holds summary information about the train route.
type RouteDetails struct {
	TrainID       string `json:"train_id"`
	Line          string `json:"line"`
	Route         string `json:"route"`
	OriginID      string `json:"origin_id"`
	DestinationID string `json:"destination_id"`
}

// RouteResponse is the payload returned by GetRoute.
type RouteResponse struct {
	Routes  []RouteStop  `json:"routes"`
	Details RouteDetails `json:"details"`
}

// GetRoute godoc
//
//	@Summary		Get train route with stop sequence
//	@Description	Returns the ordered stop list and route details for a given train ID.
//	@Tags			route
//	@Produce		json
//	@Param			train_id	path		string	true	"Train ID"
//	@Success		200			{object}	response.Response{data=RouteResponse}
//	@Failure		404			{object}	response.Response
//	@Failure		500			{object}	response.Response
//	@Router			/v1/route/{train_id} [get]
func (h *RouteHandler) GetRoute(c *gin.Context) {
	ctx := context.Background()
	trainID := c.Param("train_id")
	cacheKey := "route:" + trainID

	var routeResp RouteResponse
	if err := h.cache.Get(ctx, cacheKey, &routeResp); err == nil {
		response.BuildSuccess(c, routeResp)
		return
	}

	var schedules []models.Schedule
	if err := h.db.Where("train_id = ?", trainID).Order("departs_at asc").Find(&schedules).Error; err != nil {
		response.BuildError(c, http.StatusInternalServerError, "failed to fetch route")
		return
	}

	if len(schedules) == 0 {
		response.BuildError(c, http.StatusNotFound, "route not found")
		return
	}

	routes := make([]RouteStop, len(schedules))
	for i, s := range schedules {
		routes[i] = RouteStop{
			StationID: s.StationID,
			DepartsAt: s.DepartsAt,
			ArrivesAt: s.ArrivesAt,
		}
	}

	first := schedules[0]
	last := schedules[len(schedules)-1]
	routeResp = RouteResponse{
		Routes: routes,
		Details: RouteDetails{
			TrainID:       first.TrainID,
			Line:          first.Line,
			Route:         first.Route,
			OriginID:      first.OriginID,
			DestinationID: last.DestinationID,
		},
	}

	_ = h.cache.Set(ctx, cacheKey, routeResp, cache.TTLToMidnight())
	response.BuildSuccess(c, routeResp)
}
