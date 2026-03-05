// Package main is the entry point for the KRL Commuter Telegram Bot.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/comuline/api/internal/bot"
	"github.com/comuline/api/internal/config"
	"github.com/comuline/api/internal/database"
	"github.com/comuline/api/internal/models"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	config.SetupLogging(cfg.Env)

	if cfg.TelegramToken == "" {
		slog.Error("TELEGRAM_TOKEN is not set")
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is not set")
		os.Exit(1)
	}
	if cfg.RedisURL == "" {
		slog.Error("REDIS_URL is not set")
		os.Exit(1)
	}

	db := database.Init(cfg)

	// Run bot-specific auto-migrations in development.
	if cfg.Env != "production" {
		if err := db.AutoMigrate(&models.BotUser{}, &models.OneTimeAlert{}); err != nil {
			slog.Error("bot migration failed", "error", err)
			os.Exit(1)
		}
	}

	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("invalid REDIS_URL", "error", err)
		os.Exit(1)
	}
	rc := redis.NewClient(opt)

	b, err := bot.New(cfg, db, rc)
	if err != nil {
		slog.Error("failed to create bot", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start dispatcher (one-time alerts + daily notifications).
	dispatcher := bot.NewDispatcher(b, db)
	startDispatcher(ctx, dispatcher, cfg.Timezone)

	// Run the bot (blocks until ctx cancelled).
	go b.Run(ctx)

	// Wait for OS signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down bot")
	cancel()
}

// startDispatcher launches background goroutines for:
// - One-time alert polling (every minute)
// - Morning push notification (at 06:45 WIB on work days)
// - Evening push notification (at 17:15 WIB on work days)
func startDispatcher(ctx context.Context, d *bot.Dispatcher, tz string) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.FixedZone("WIB", 7*60*60)
	}
	// One-time alert scanner — runs every minute.
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.DispatchOneTimeAlerts(ctx)
			}
		}
	}()

	// Daily push notifications — run at configured times.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			now := time.Now().In(loc)
			nextMorning := nextDailyTime(now, loc, 6, 45)
			nextEvening := nextDailyTime(now, loc, 17, 15)

			var next time.Time
			isMorning := false
			if nextMorning.Before(nextEvening) {
				next = nextMorning
				isMorning = true
			} else {
				next = nextEvening
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
				d.DispatchDailyPushNotifications(ctx, isMorning)
			}
		}
	}()
}

func nextDailyTime(now time.Time, loc *time.Location, hour, minute int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	return t
}
