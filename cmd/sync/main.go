package main

import (
	"flag"
	"log"

	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/database"
	syncer "github.com/comuline/api/internal/sync"
)

func main() {
	syncType := flag.String("type", "", "sync type: station|schedule|mrt-station|mrt-schedule|mrt")
	flag.Parse()

	if *syncType == "" {
		log.Fatal("--type is required (station|schedule|mrt-station|mrt-schedule|mrt)")
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
	case "mrt-station":
		if err := syncer.SyncMRTStations(db); err != nil {
			log.Fatalf("MRT station sync failed: %v", err)
		}
	case "mrt-schedule":
		if err := syncer.SyncMRTSchedules(db); err != nil {
			log.Fatalf("MRT schedule sync failed: %v", err)
		}
	case "mrt":
		if err := syncer.SyncMRTStations(db); err != nil {
			log.Fatalf("MRT station sync failed: %v", err)
		}
		if err := syncer.SyncMRTSchedules(db); err != nil {
			log.Fatalf("MRT schedule sync failed: %v", err)
		}
	default:
		log.Fatalf("unknown sync type %q — use station|schedule|mrt-station|mrt-schedule|mrt", *syncType)
	}

	log.Println("sync completed successfully")
}
