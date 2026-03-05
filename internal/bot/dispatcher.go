// Package bot provides the notification dispatcher for scheduled alerts.
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/comuline/api/internal/bot/i18n"
	"github.com/comuline/api/internal/models"
	"gorm.io/gorm"
)

// Dispatcher sends push notifications and one-time alerts.
type Dispatcher struct {
	bot *Bot
	db  *gorm.DB
}

// NewDispatcher creates a Dispatcher.
func NewDispatcher(bot *Bot, db *gorm.DB) *Dispatcher {
	return &Dispatcher{bot: bot, db: db}
}

// DispatchOneTimeAlerts checks for pending one-time alerts and sends them.
func (d *Dispatcher) DispatchOneTimeAlerts(ctx context.Context) {
	payloads, err := PendingAlerts(ctx, d.bot.rc)
	if err != nil {
		slog.Error("failed to fetch pending alerts", "error", err)
		return
	}
	if len(payloads) == 0 {
		return
	}

	slog.Info("dispatching one-time alerts", "count", len(payloads))
	loc := d.bot.loc

	var membersToRemove []string

	for _, p := range payloads {
		data, _ := json.Marshal(p)
		membersToRemove = append(membersToRemove, string(data))

		// Get user for language preference.
		var user models.BotUser
		if err := d.db.WithContext(ctx).First(&user, "telegram_id = ?", p.TelegramID).Error; err != nil {
			slog.Warn("alert: user not found", "telegram_id", p.TelegramID)
			continue
		}

		origin := &stationInfo{Name: p.OriginName, Code: p.OriginCode, Lat: p.OriginLat, Lon: p.OriginLon}
		dest := &stationInfo{Name: p.DestName, Code: p.DestCode}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("🔔 *SCHEDULED ALERT*: %s → %s\n\n", escapeMarkdown(p.OriginName), escapeMarkdown(p.DestName)))

		// Weather.
		if origin.Lat != 0 && origin.Lon != 0 {
			f, err := d.bot.weather.Get(ctx, origin.Lat, origin.Lon)
			if err == nil {
				sb.WriteString(fmt.Sprintf("%s %.0f°C %s, feels %.0f°C\n\n", f.Emoji(), f.Temp, f.Condition, f.FeelsLike))
			}
		}

		// Top 3 trains.
		now := time.Now().In(loc)
		windowEnd := now.Add(2 * time.Hour)
		schedules, err := d.bot.getSchedulesBetween(ctx, origin.Code, dest.Code, now, windowEnd)
		if err == nil && len(schedules) > 0 {
			sb.WriteString("Top trains:\n")
			for i, s := range schedules {
				if i >= 3 {
					break
				}
				dep := s.DepartsAt.In(loc).Format("15:04")
				arr := s.ArrivesAt.In(loc).Format("15:04")
				sb.WriteString(fmt.Sprintf("• %s→%s %s\n", dep, arr, escapeMarkdown(s.Line)))
			}
		}
		sb.WriteString("\n_Reply /schedule for full list._")

		d.bot.SendMessage(p.TelegramID, sb.String())

		// Mark as sent in DB.
		now2 := time.Now()
		success := true
		d.db.WithContext(ctx).Model(&models.OneTimeAlert{}).
			Where("id = ?", p.ID).
			Updates(map[string]interface{}{"sent_at": now2, "success": success})
	}

	// Remove processed alerts from Redis.
	if len(membersToRemove) > 0 {
		if err := RemoveAlerts(ctx, d.bot.rc, membersToRemove); err != nil {
			slog.Error("failed to remove processed alerts from redis", "error", err)
		}
	}
}

// DispatchDailyPushNotifications sends morning or evening notifications to users
// who have notifications enabled and are currently within their work schedule window.
func (d *Dispatcher) DispatchDailyPushNotifications(ctx context.Context, morning bool) {
	var users []models.BotUser
	if err := d.db.WithContext(ctx).Where("notifications = true").Find(&users).Error; err != nil {
		slog.Error("failed to fetch notification users", "error", err)
		return
	}

	loc := d.bot.loc
	now := time.Now().In(loc)
	dayAbbrev := strings.ToLower(now.Weekday().String()[:3])

	slog.Info("dispatching daily notifications", "morning", morning, "users", len(users))

	for _, u := range users {
		var days []string
		_ = json.Unmarshal(u.WorkDays, &days)
		if !containsDay(days, dayAbbrev) {
			continue
		}

		home := d.bot.stationFromJSON(u.HomeStation)
		away := d.bot.stationFromJSON(u.AwayStation)
		if home == nil || away == nil {
			continue
		}

		msgs := i18n.New(u.Lang)
		if morning {
			d.bot.sendScheduleWithWeather(ctx, u.TelegramID, home, away, now, msgs)
		} else {
			d.bot.sendScheduleWithWeather(ctx, u.TelegramID, away, home, now, msgs)
		}
	}
}

func containsDay(days []string, day string) bool {
	for _, d := range days {
		if d == day {
			return true
		}
	}
	return false
}

