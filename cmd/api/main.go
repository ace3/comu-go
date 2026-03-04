// @title           Comuline API
// @version         1.0
// @description     Indonesian KRL Commuter Line Schedule API
// @host            localhost:8080
// @BasePath        /
package main

import (
	"log"
	"net/http"

	_ "github.com/comuline/api/docs"
	"github.com/comuline/api/internal/cache"
	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/database"
	"github.com/comuline/api/internal/handlers"
	"github.com/comuline/api/internal/middleware"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
	cfg := config.Load()
	db := database.Init(cfg)
	c := cache.New(cfg.RedisURL)

	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.Use(middleware.CORS())

	// Root redirect → /docs
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/docs/index.html")
	})

	// Health check
	r.GET("/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Swagger UI
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Raw OpenAPI spec
	r.GET("/openapi", func(c *gin.Context) {
		c.File("./docs/swagger.json")
	})

	// API v1
	v1 := r.Group("/v1")
	{
		stationH := handlers.NewStationHandler(db, c)
		v1.GET("/station", stationH.GetStations)
		v1.GET("/station/:id", stationH.GetStation)

		scheduleH := handlers.NewScheduleHandler(db, c)
		v1.GET("/schedule/:station_id", scheduleH.GetSchedules)

		routeH := handlers.NewRouteHandler(db, c)
		v1.GET("/route/:train_id", routeH.GetRoute)
	}

	log.Printf("starting Comuline API on :%s (env: %s)", cfg.Port, cfg.Env)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
