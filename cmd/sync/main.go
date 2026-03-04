package main

import (
	"flag"
	"log"

	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/database"
	syncer "github.com/comuline/api/internal/sync"
)

func main() {
	syncType := flag.String("type", "", "sync type: station|schedule")
	flag.Parse()

	if *syncType == "" {
		log.Fatal("--type is required (station|schedule)")
	}

	cfg := config.Load()
	db := database.Init(cfg)

	switch *syncType {
	case "station":
		if err := syncer.SyncStations(cfg, db); err != nil {
			log.Fatalf("station sync failed: %v", err)
		}
	case "schedule":
		if err := syncer.SyncSchedules(cfg, db); err != nil {
			log.Fatalf("schedule sync failed: %v", err)
		}
	default:
		log.Fatalf("unknown sync type %q — use station or schedule", *syncType)
	}

	log.Println("sync completed successfully")
}
