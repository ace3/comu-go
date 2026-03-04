// @title           Comuline API
// @version         1.0
// @description     Indonesian KRL Commuter Line Schedule API
// @host            localhost:8080
// @BasePath        /
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/comuline/api/docs"
	"github.com/comuline/api/internal/cache"
	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/database"
	"github.com/comuline/api/internal/handlers"
	"github.com/comuline/api/internal/middleware"
	"github.com/comuline/api/internal/scheduler"
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

	// Manual sync trigger
	r.POST("/sync", func(c *gin.Context) {
		go func() {
			if err := scheduler.RunNow(cfg, db); err != nil {
				log.Printf("manual sync error: %v", err)
			}
		}()
		c.JSON(http.StatusAccepted, gin.H{"message": "sync started"})
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

		mrtH := handlers.NewMRTHandler(db, c)
		mrt := v1.Group("/mrt")
		{
			mrt.GET("/stations", mrtH.GetMRTStations)
			mrt.GET("/stations/:id", mrtH.GetMRTStation)
			mrt.GET("/schedules/:station_id", mrtH.GetMRTSchedules)
			mrt.GET("/routes", mrtH.GetMRTRoutes)
		}
	}

	// Start background scheduler (stops when ctx is cancelled)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler.Start(ctx, cfg, db)

	// HTTP server with graceful shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("starting Comuline API on :%s (env: %s)", cfg.Port, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")
	cancel() // stop scheduler

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("server stopped")
}
