package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/database"
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		slog.Error("usage: migrate [up|down]")
		os.Exit(1)
	}

	cfg := config.Load()
	config.SetupLogging(cfg.Env)

	switch args[0] {
	case "up":
		database.RunMigrations(cfg.DatabaseURL)
	case "down":
		database.RunMigrationsDown(cfg.DatabaseURL)
	default:
		slog.Error("unknown command", "command", args[0])
		os.Exit(1)
	}
}
