// @title           Comu API
// @version         1.0
// @description     Indonesian KRL Commuter Line Schedule API
// @host            api.comu.com
// @BasePath        /
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/comu/api/internal/cache"
	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/database"
	"github.com/comu/api/internal/handlers"
	"github.com/comu/api/internal/middleware"
	"github.com/comu/api/internal/scheduler"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
	cfg := config.Load()
	config.SetupLogging(cfg.Env)

	if err := cfg.Validate(); err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	db := database.Init(cfg)
	c := cache.New(cfg.RedisURL)
	if err := backfillFromDataIfEmpty(db, os.Getenv("BACKFILL_DATA_DIR")); err != nil {
		slog.Error("startup backfill failed", "error", err)
	}
	if err := ensureInitialStationData(cfg, db, c, scheduler.RunNow); err != nil {
		slog.Error("initial station sync failed", "error", err)
	}
	c.InvalidateAll(context.Background())

	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.Use(middleware.CORS())
	r.Use(middleware.RequestID())
	r.Use(middleware.MetricsMiddleware())

	// Root redirect → /app
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/app")
	})
	r.GET("/app", func(c *gin.Context) {
		c.File("./web/index.html")
	})
	r.Static("/app/assets", "./web")

	// Health check with real dependency pings
	r.GET("/status", func(gc *gin.Context) {
		ctx, cancel := context.WithTimeout(gc.Request.Context(), 500*time.Millisecond)
		defer cancel()

		status := http.StatusOK
		result := gin.H{}

		// Ping Postgres
		sqlDB, err := db.DB()
		if err != nil {
			result["postgres"] = "error: " + err.Error()
			status = http.StatusServiceUnavailable
		} else if err := sqlDB.PingContext(ctx); err != nil {
			result["postgres"] = "error: " + err.Error()
			status = http.StatusServiceUnavailable
		} else {
			result["postgres"] = "ok"
		}

		// Ping Redis
		if err := c.Client().Ping(ctx).Err(); err != nil {
			result["redis"] = "error: " + err.Error()
			status = http.StatusServiceUnavailable
		} else {
			result["redis"] = "ok"
		}

		if status == http.StatusOK {
			result["status"] = "ok"
		} else {
			result["status"] = "degraded"
		}

		gc.JSON(status, result)
	})

	// Swagger UI
	r.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/openapi")))

	// Raw OpenAPI spec
	r.GET("/openapi", func(c *gin.Context) {
		scheme := requestScheme(c.Request)
		doc, err := buildDynamicSwaggerDoc(c.Request.Host, scheme)
		if err != nil {
			slog.Error("failed to render dynamic openapi document", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to render openapi"})
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", doc)
	})

	// Prometheus metrics
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Manual sync trigger (authenticated)
	syncGroup := r.Group("/")
	if cfg.SyncSecret != "" {
		syncGroup.Use(middleware.RequireSyncSecret(cfg.SyncSecret))
	}
	syncGroup.POST("/sync", func(gc *gin.Context) {
		go func() {
			if err := scheduler.RunNow(cfg, db, c); err != nil {
				slog.Error("manual sync error", "error", err)
			}
		}()
		gc.JSON(http.StatusAccepted, gin.H{"message": "sync started"})
	})

	// Force backfill from local JSON files while app is running.
	// Protected by the same X-Sync-Secret header as /sync.
	syncGroup.POST("/admin/backfill", func(gc *gin.Context) {
		dataDir := gc.Query("data_dir")
		go func() {
			if err := backfillFromDataForce(db, dataDir); err != nil {
				slog.Error("manual json backfill error", "error", err, "data_dir", dataDir)
				return
			}
			c.InvalidateAll(context.Background())
			slog.Info("manual json backfill complete", "data_dir", dataDir)
		}()
		gc.JSON(http.StatusAccepted, gin.H{"message": "json backfill started"})
	})

	// Token rotation — update KAI_AUTH_TOKEN in memory without restarting.
	// Protected by the same X-Sync-Secret header as /sync.
	syncGroup.POST("/admin/rotate-token", func(gc *gin.Context) {
		var body struct {
			Token string `json:"token" binding:"required"`
		}
		if err := gc.ShouldBindJSON(&body); err != nil {
			gc.JSON(http.StatusBadRequest, gin.H{"error": "token field required"})
			return
		}
		cfg.RotateToken(body.Token)
		gc.JSON(http.StatusOK, gin.H{"message": "token rotated"})
	})

	// API v1
	v1 := r.Group("/v1")
	{
		stationH := handlers.NewStationHandler(db, c)
		v1.GET("/station", stationH.GetStations)
		v1.GET("/station/:id", stationH.GetStation)

		scheduleH := handlers.NewScheduleHandler(db, c)
		v1.GET("/schedule/:station_id", scheduleH.GetSchedules)
		v1.GET("/schedule/window", scheduleH.GetScheduleWindow)

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
	if cfg.AutoSync {
		scheduler.Start(ctx, cfg, db, c)
	}

	// HTTP server with graceful shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	go func() {
		slog.Info("starting Comu API", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	cancel() // stop scheduler

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
