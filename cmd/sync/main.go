package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/database"
	syncer "github.com/comuline/api/internal/sync"
)

func main() {
	syncType := flag.String("type", "", "sync type: station|schedule")
	flag.Parse()

	if *syncType == "" {
		slog.Error("--type is required (station|schedule)")
		os.Exit(1)
	}

	cfg := config.Load()
	config.SetupLogging(cfg.Env)

	if err := cfg.Validate(); err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	db := database.Init(cfg)

	switch *syncType {
	case "station":
		if err := syncer.SyncStations(cfg, db); err != nil {
			slog.Error("station sync failed", "error", err)
			os.Exit(1)
		}
	case "schedule":
		if err := syncer.SyncSchedules(cfg, db); err != nil {
			slog.Error("schedule sync failed", "error", err)
			os.Exit(1)
		}
	default:
		slog.Error("unknown sync type", "type", *syncType)
		os.Exit(1)
	}

	slog.Info("sync completed successfully")
}
