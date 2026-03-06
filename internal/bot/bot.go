// Package bot implements the KRL Commuter Telegram Bot.
package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/comu/api/internal/bot/i18n"
	"github.com/comu/api/internal/bot/session"
	"github.com/comu/api/internal/bot/weather"
	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/handlers"
	"github.com/comu/api/internal/models"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Bot is the main Telegram bot handler.
type Bot struct {
	api      *tgbotapi.BotAPI
	client   telegramClient
	db       *gorm.DB
	rc       *redis.Client
	sessions *session.Store
	weather  *weather.Client
	cfg      *config.Config
	loc      *time.Location
}

type telegramClient interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

type botTripPlanResponse struct {
	Data struct {
		Options []struct {
			DepartAt string `json:"departAt"`
			ArriveAt string `json:"arriveAt"`
			Legs     []struct {
				TrainID  string `json:"trainId"`
				Line     string `json:"line"`
				From     string `json:"from"`
				To       string `json:"to"`
				DepartAt string `json:"departAt"`
				ArriveAt string `json:"arriveAt"`
			} `json:"legs"`
		} `json:"options"`
	} `json:"data"`
}

// New creates a new Bot instance.
func New(cfg *config.Config, db *gorm.DB, rc *redis.Client) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("bot: failed to create Telegram API: %w", err)
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("bot: invalid timezone %q: %w", cfg.Timezone, err)
	}
	return &Bot{
		api:      api,
		client:   api,
		db:       db,
		rc:       rc,
		sessions: session.New(rc),
		weather:  weather.New(cfg.OpenMeteoBase),
		cfg:      cfg,
		loc:      loc,
	}, registerBotCommands(api)
}

// API returns the underlying Telegram bot API.
func (b *Bot) API() *tgbotapi.BotAPI { return b.api }

func registerBotCommands(api *tgbotapi.BotAPI) error {
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Open the main commuter menu"},
		tgbotapi.BotCommand{Command: "go_morning", Description: "Home to away schedule"},
		tgbotapi.BotCommand{Command: "go_evening", Description: "Away to home schedule"},
		tgbotapi.BotCommand{Command: "schedule", Description: "Manual station-to-station schedule"},
		tgbotapi.BotCommand{Command: "schedule_once", Description: "Create a one-time alert"},
		tgbotapi.BotCommand{Command: "list_alerts", Description: "List scheduled alerts"},
		tgbotapi.BotCommand{Command: "settings", Description: "View your saved profile"},
		tgbotapi.BotCommand{Command: "help", Description: "Show all commands"},
	)
	_, err := api.Request(commands)
	if err != nil {
		return fmt.Errorf("bot: register commands: %w", err)
	}
	return nil
}

// Run starts the bot polling loop. It stops when ctx is cancelled.
func (b *Bot) Run(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)

	slog.Info("bot started", "username", b.api.Self.UserName)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			slog.Info("bot stopped")
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			go b.handleUpdate(ctx, update)
		}
	}
}

// handleUpdate routes a single Telegram update.
func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		query := update.CallbackQuery
		user, err := b.getOrCreateUser(ctx, query.From.ID, query.From.FirstName)
		if err != nil {
			slog.Error("failed to get/create user for callback", "error", err)
			return
		}
		msgs := i18n.New(user.Lang)
		b.handleCallbackQuery(ctx, query, user, msgs)
		return
	}

	if update.Message == nil {
		return
	}
	msg := update.Message
	userID := msg.From.ID
	firstName := msg.From.FirstName

	user, err := b.getOrCreateUser(ctx, userID, firstName)
	if err != nil {
		slog.Error("failed to get/create user", "error", err)
		return
	}

	msgs := i18n.New(user.Lang)

	// Check if user is in the middle of a conversation.
	st, err := b.sessions.Get(ctx, userID)
	if err != nil {
		slog.Error("session get error", "error", err)
	}

	if msg.IsCommand() {
		// New command always resets any in-progress session.
		_ = b.sessions.Clear(ctx, userID)
		b.handleCommand(ctx, msg, user, msgs)
		return
	}

	// Handle text reply for active session.
	if st != nil {
		b.handleSessionReply(ctx, msg, user, msgs, st)
		return
	}

	if b.handleMenuText(ctx, msg, user, msgs) {
		return
	}

	// Unknown free-form text - show help.
	b.showMainMenu(ctx, msg.Chat.ID, msgs, msgs.Help())
}

// handleCommand routes a slash command.
func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	cmd := msg.Command()
	switch cmd {
	case "start":
		b.showMainMenu(ctx, msg.Chat.ID, msgs, msgs.Welcome(msg.From.FirstName))
	case "help":
		b.showMainMenu(ctx, msg.Chat.ID, msgs, msgs.Help())
	case "set_route":
		b.startSetRoute(ctx, msg, msgs)
	case "set_schedule":
		b.startSetSchedule(ctx, msg, user, msgs)
	case "toggle_notifs":
		b.handleToggleNotifs(ctx, msg, user, msgs)
	case "go_morning":
		b.handleGoMorning(ctx, msg, user, msgs)
	case "go_evening":
		b.handleGoEvening(ctx, msg, user, msgs)
	case "schedule":
		b.startSchedule(ctx, msg, msgs)
	case "schedule_once":
		b.startScheduleOnce(ctx, msg, msgs)
	case "list_alerts":
		b.handleListAlerts(ctx, msg, user, msgs)
	case "cancel_alert":
		b.handleCancelAlert(ctx, msg, user, msgs)
	case "station":
		b.handleStationSearch(ctx, msg, msgs)
	case "settings":
		b.handleSettings(ctx, msg, user, msgs)
	case "lang":
		b.handleLang(ctx, msg, user, msgs)
	default:
		b.send(msg.Chat.ID, msgs.Help())
	}
}

// ---- Command Implementations ----

func (b *Bot) startSetRoute(ctx context.Context, msg *tgbotapi.Message, msgs *i18n.Messages) {
	b.startSetRouteForUser(ctx, msg.From.ID, msg.Chat.ID, msgs)
}

func (b *Bot) startSetRouteForUser(ctx context.Context, userID, chatID int64, msgs *i18n.Messages) {
	if b.sessions != nil {
		_ = b.sessions.Set(ctx, userID, &session.State{
			Command: "set_route",
			Step:    1,
			Data:    map[string]string{},
		})
	}
	b.sendMarkdown(chatID, msgs.AskHomeStation())
}

func (b *Bot) startSetSchedule(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	b.startSetScheduleForUser(ctx, msg.From.ID, msg.Chat.ID, user, msgs)
}

func (b *Bot) startSetScheduleForUser(ctx context.Context, userID, chatID int64, user *models.BotUser, msgs *i18n.Messages) {
	var days []string
	_ = json.Unmarshal(user.WorkDays, &days)
	if b.sessions != nil {
		_ = b.sessions.Set(ctx, userID, &session.State{
			Command: "set_schedule",
			Step:    1,
			Data:    map[string]string{},
		})
	}
	b.sendMarkdown(chatID, msgs.WorkSchedulePrompt(days))
}

func (b *Bot) handleToggleNotifs(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	user.Notifications = !user.Notifications
	if err := b.db.WithContext(ctx).Save(user).Error; err != nil {
		b.send(msg.Chat.ID, msgs.InternalError())
		return
	}
	if user.Notifications {
		b.sendMarkdown(msg.Chat.ID, msgs.NotificationsOn())
	} else {
		b.sendMarkdown(msg.Chat.ID, msgs.NotificationsOff())
	}
}

func (b *Bot) handleGoMorning(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	home, away := b.stationFromJSON(user.HomeStation), b.stationFromJSON(user.AwayStation)
	if home == nil || away == nil {
		b.sendMarkdown(msg.Chat.ID, msgs.NoRouteSet())
		return
	}
	loc := b.loc
	now := time.Now().In(loc)
	b.sendScheduleWithWeather(ctx, msg.Chat.ID, home, away, now, msgs)
	b.showCommuteActions(ctx, msg.Chat.ID, user.TelegramID, "morning", msgs)
}

func (b *Bot) handleGoEvening(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	home, away := b.stationFromJSON(user.HomeStation), b.stationFromJSON(user.AwayStation)
	if home == nil || away == nil {
		b.sendMarkdown(msg.Chat.ID, msgs.NoRouteSet())
		return
	}
	loc := b.loc
	now := time.Now().In(loc)
	b.sendScheduleWithWeather(ctx, msg.Chat.ID, away, home, now, msgs)
	b.showCommuteActions(ctx, msg.Chat.ID, user.TelegramID, "evening", msgs)
}

func (b *Bot) startSchedule(ctx context.Context, msg *tgbotapi.Message, msgs *i18n.Messages) {
	b.startScheduleForUser(ctx, msg.From.ID, msg.Chat.ID, strings.TrimSpace(msg.CommandArguments()), msgs)
}

func (b *Bot) startScheduleForUser(ctx context.Context, userID, chatID int64, inlineOrigin string, msgs *i18n.Messages) {
	st := &session.State{
		Command: "schedule",
		Step:    1,
		Data:    map[string]string{},
	}
	if b.sessions != nil {
		_ = b.sessions.Set(ctx, userID, st)
	}

	// If origin was passed inline (e.g. /schedule rawa buaya), skip step 1.
	if inlineOrigin != "" {
		msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}, From: &tgbotapi.User{ID: userID}}
		b.handleScheduleStep(ctx, msg, nil, msgs, st, inlineOrigin)
		return
	}
	b.sendMarkdown(chatID, msgs.AskScheduleOrigin())
}

func (b *Bot) startScheduleOnce(ctx context.Context, msg *tgbotapi.Message, msgs *i18n.Messages) {
	b.startScheduleOnceForUser(ctx, msg.From.ID, msg.Chat.ID, msgs)
}

func (b *Bot) startScheduleOnceForUser(ctx context.Context, userID, chatID int64, msgs *i18n.Messages) {
	if b.sessions != nil {
		_ = b.sessions.Set(ctx, userID, &session.State{
			Command: "schedule_once",
			Step:    1,
			Data:    map[string]string{},
		})
	}
	b.sendMarkdown(chatID, msgs.AskAlertOrigin())
}

func (b *Bot) handleListAlerts(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	alerts, err := ListUserAlerts(ctx, b.db, user.TelegramID)
	if err != nil {
		b.send(msg.Chat.ID, msgs.InternalError())
		return
	}
	if len(alerts) == 0 {
		b.sendMarkdown(msg.Chat.ID, msgs.NoAlerts())
		return
	}

	loc := b.loc
	lines := []string{}
	for _, a := range alerts {
		var orig, dest stationInfo
		_ = json.Unmarshal(a.Origin, &orig)
		_ = json.Unmarshal(a.Dest, &dest)
		t := a.ScheduledFor.In(loc).Format("02 Jan 15:04")
		lines = append(lines, fmt.Sprintf("• %s: %s→%s [/cancel\\_alert %s]", t, orig.Name, dest.Name, a.ID[:8]))
	}
	b.sendMarkdown(msg.Chat.ID, "🔔 *Scheduled Alerts:*\n"+strings.Join(lines, "\n"))
}

func (b *Bot) handleCancelAlert(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	args := msg.CommandArguments()
	if args == "" {
		b.send(msg.Chat.ID, "Usage: /cancel_alert <id>")
		return
	}
	idPrefix := strings.TrimSpace(args)
	if err := CancelAlert(ctx, b.rc, b.db, user.TelegramID, idPrefix); err != nil {
		b.sendMarkdown(msg.Chat.ID, msgs.AlertNotFound(idPrefix))
		return
	}
	b.sendMarkdown(msg.Chat.ID, msgs.AlertCancelled(idPrefix))
}

func (b *Bot) handleStationSearch(ctx context.Context, msg *tgbotapi.Message, msgs *i18n.Messages) {
	query := msg.CommandArguments()
	if query == "" {
		b.send(msg.Chat.ID, "Usage: /station <name>")
		return
	}

	stations, err := b.getAllKRLStations(ctx)
	if err != nil {
		b.send(msg.Chat.ID, msgs.InternalError())
		return
	}

	results := FindStation(stations, query)
	if len(results) == 0 {
		b.sendMarkdown(msg.Chat.ID, msgs.StationNotFound(query))
		return
	}

	lines := []string{}
	for i, s := range results {
		if i >= 10 {
			break
		}
		lines = append(lines, fmt.Sprintf("%d. %s (%s)", i+1, s.Name, s.ID))
	}
	b.send(msg.Chat.ID, "🚉 Stations:\n"+strings.Join(lines, "\n"))
}

func (b *Bot) handleSettings(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	home, away := b.stationFromJSON(user.HomeStation), b.stationFromJSON(user.AwayStation)
	homeName, awayName := "-", "-"
	if home != nil {
		homeName = home.Name
	}
	if away != nil {
		awayName = away.Name
	}
	var days []string
	_ = json.Unmarshal(user.WorkDays, &days)
	b.sendMarkdown(msg.Chat.ID, msgs.Settings(user.TelegramID, homeName, awayName, user.MorningTime, user.EveningTime, days, user.Notifications, user.Lang))
}

func (b *Bot) handleLang(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) {
	if user.Lang == "en" {
		user.Lang = "id"
	} else {
		user.Lang = "en"
	}
	if err := b.db.WithContext(ctx).Save(user).Error; err != nil {
		b.send(msg.Chat.ID, msgs.InternalError())
		return
	}
	newMsgs := i18n.New(user.Lang)
	b.sendMarkdown(msg.Chat.ID, newMsgs.LangSwitched())
}

// ---- Session Reply Handler ----

func (b *Bot) handleSessionReply(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages, st *session.State) {
	text := strings.TrimSpace(msg.Text)
	switch st.Command {
	case "set_route":
		b.handleSetRouteStep(ctx, msg, user, msgs, st, text)
	case "set_schedule":
		b.handleSetScheduleStep(ctx, msg, user, msgs, st, text)
	case "schedule":
		b.handleScheduleStep(ctx, msg, user, msgs, st, text)
	case "schedule_once":
		b.handleScheduleOnceStep(ctx, msg, user, msgs, st, text)
	}
}

func (b *Bot) handleSetRouteStep(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages, st *session.State, text string) {
	switch st.Step {
	case 1: // Asking home station
		stations, _ := b.getAllKRLStations(ctx)
		results := FindStation(stations, text)
		if len(results) == 0 {
			b.sendMarkdown(msg.Chat.ID, msgs.StationNotFound(text))
			return
		}
		if len(results) > 1 {
			var names []string
			for _, s := range results {
				names = append(names, s.Name+" ("+s.ID+")")
			}
			b.sendMarkdown(msg.Chat.ID, msgs.MultipleStationsFound(text, names))
			return
		}
		s := results[0]
		st.Data["home_name"] = s.Name
		st.Data["home_code"] = s.ID
		st.Data["home_lat"] = fmt.Sprintf("%f", latFromStation(s))
		st.Data["home_lon"] = fmt.Sprintf("%f", lonFromStation(s))
		st.Step = 2
		_ = b.sessions.Set(ctx, msg.From.ID, st)
		b.sendMarkdown(msg.Chat.ID, msgs.AskAwayStation())

	case 2: // Asking away station
		stations, _ := b.getAllKRLStations(ctx)
		results := FindStation(stations, text)
		if len(results) == 0 {
			b.sendMarkdown(msg.Chat.ID, msgs.StationNotFound(text))
			return
		}
		if len(results) > 1 {
			var names []string
			for _, s := range results {
				names = append(names, s.Name+" ("+s.ID+")")
			}
			b.sendMarkdown(msg.Chat.ID, msgs.MultipleStationsFound(text, names))
			return
		}
		s := results[0]
		st.Data["away_name"] = s.Name
		st.Data["away_code"] = s.ID
		st.Data["away_lat"] = fmt.Sprintf("%f", latFromStation(s))
		st.Data["away_lon"] = fmt.Sprintf("%f", lonFromStation(s))
		st.Step = 3
		_ = b.sessions.Set(ctx, msg.From.ID, st)
		b.sendMarkdown(msg.Chat.ID, msgs.AskMorningTime())

	case 3: // Asking morning time
		if !isValidTime(text) {
			b.sendMarkdown(msg.Chat.ID, msgs.InvalidTime())
			return
		}
		st.Data["morning"] = text
		st.Step = 4
		_ = b.sessions.Set(ctx, msg.From.ID, st)
		b.sendMarkdown(msg.Chat.ID, msgs.AskEveningTime())

	case 4: // Asking evening time
		if !isValidTime(text) {
			b.sendMarkdown(msg.Chat.ID, msgs.InvalidTime())
			return
		}
		st.Data["evening"] = text

		// Save to DB
		homeJSON, _ := json.Marshal(stationInfo{Name: st.Data["home_name"], Code: st.Data["home_code"]})
		awayJSON, _ := json.Marshal(stationInfo{Name: st.Data["away_name"], Code: st.Data["away_code"]})
		user.HomeStation = datatypes.JSON(homeJSON)
		user.AwayStation = datatypes.JSON(awayJSON)
		user.MorningTime = st.Data["morning"]
		user.EveningTime = st.Data["evening"]
		if err := b.db.WithContext(ctx).Save(user).Error; err != nil {
			b.send(msg.Chat.ID, msgs.InternalError())
			return
		}
		_ = b.sessions.Clear(ctx, msg.From.ID)
		b.sendMarkdown(msg.Chat.ID, msgs.RouteSet(st.Data["home_name"], st.Data["away_name"], st.Data["morning"], st.Data["evening"]))
	}
}

func (b *Bot) handleSetScheduleStep(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages, st *session.State, text string) {
	days := parseWorkDays(text)
	if len(days) == 0 {
		b.send(msg.Chat.ID, "❌ Invalid days. Use: mon,tue,wed,thu,fri")
		return
	}
	daysJSON, _ := json.Marshal(days)
	user.WorkDays = datatypes.JSON(daysJSON)
	if err := b.db.WithContext(ctx).Save(user).Error; err != nil {
		b.send(msg.Chat.ID, msgs.InternalError())
		return
	}
	_ = b.sessions.Clear(ctx, msg.From.ID)
	b.sendMarkdown(msg.Chat.ID, msgs.WorkScheduleSet(days))
}

func (b *Bot) handleScheduleStep(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages, st *session.State, text string) {
	switch st.Step {
	case 1: // origin
		stations, _ := b.getAllKRLStations(ctx)
		results := FindStation(stations, text)
		if len(results) == 0 {
			b.sendMarkdown(msg.Chat.ID, msgs.StationNotFound(text))
			return
		}
		if len(results) > 1 {
			var names []string
			for _, s := range results {
				names = append(names, s.Name+" ("+s.ID+")")
			}
			b.sendMarkdown(msg.Chat.ID, msgs.MultipleStationsFound(text, names))
			return
		}
		s := results[0]
		st.Data["origin_name"] = s.Name
		st.Data["origin_code"] = s.ID
		st.Data["origin_lat"] = fmt.Sprintf("%f", latFromStation(s))
		st.Data["origin_lon"] = fmt.Sprintf("%f", lonFromStation(s))
		st.Step = 2
		_ = b.sessions.Set(ctx, msg.From.ID, st)
		b.sendMarkdown(msg.Chat.ID, msgs.AskScheduleDest())

	case 2: // destination
		stations, _ := b.getAllKRLStations(ctx)
		results := FindStation(stations, text)
		if len(results) == 0 {
			b.sendMarkdown(msg.Chat.ID, msgs.StationNotFound(text))
			return
		}
		if len(results) > 1 {
			var names []string
			for _, s := range results {
				names = append(names, s.Name+" ("+s.ID+")")
			}
			b.sendMarkdown(msg.Chat.ID, msgs.MultipleStationsFound(text, names))
			return
		}
		s := results[0]
		st.Data["dest_name"] = s.Name
		st.Data["dest_code"] = s.ID
		st.Step = 3
		_ = b.sessions.Set(ctx, msg.From.ID, st)
		b.sendMarkdown(msg.Chat.ID, msgs.AskScheduleTime())

	case 3: // time
		loc := b.loc
		t, err := parseTimeInput(text, loc)
		if err != nil {
			b.sendMarkdown(msg.Chat.ID, msgs.InvalidTime())
			return
		}

		origin := &stationInfo{Name: st.Data["origin_name"], Code: st.Data["origin_code"]}
		var lat, lon float64
		fmt.Sscanf(st.Data["origin_lat"], "%f", &lat)
		fmt.Sscanf(st.Data["origin_lon"], "%f", &lon)
		origin.Lat = lat
		origin.Lon = lon

		dest := &stationInfo{Name: st.Data["dest_name"], Code: st.Data["dest_code"]}

		_ = b.sessions.Clear(ctx, msg.From.ID)
		b.sendScheduleWithWeatherRaw(ctx, msg.Chat.ID, origin, dest, t, msgs)
	}
}

func (b *Bot) handleScheduleOnceStep(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages, st *session.State, text string) {
	switch st.Step {
	case 1: // origin
		stations, _ := b.getAllKRLStations(ctx)
		results := FindStation(stations, text)
		if len(results) == 0 {
			b.sendMarkdown(msg.Chat.ID, msgs.StationNotFound(text))
			return
		}
		if len(results) > 1 {
			var names []string
			for _, s := range results {
				names = append(names, s.Name+" ("+s.ID+")")
			}
			b.sendMarkdown(msg.Chat.ID, msgs.MultipleStationsFound(text, names))
			return
		}
		s := results[0]
		st.Data["origin_name"] = s.Name
		st.Data["origin_code"] = s.ID
		st.Data["origin_lat"] = fmt.Sprintf("%f", latFromStation(s))
		st.Data["origin_lon"] = fmt.Sprintf("%f", lonFromStation(s))
		st.Step = 2
		_ = b.sessions.Set(ctx, msg.From.ID, st)
		b.sendMarkdown(msg.Chat.ID, msgs.AskAlertDest())

	case 2: // destination
		stations, _ := b.getAllKRLStations(ctx)
		results := FindStation(stations, text)
		if len(results) == 0 {
			b.sendMarkdown(msg.Chat.ID, msgs.StationNotFound(text))
			return
		}
		if len(results) > 1 {
			var names []string
			for _, s := range results {
				names = append(names, s.Name+" ("+s.ID+")")
			}
			b.sendMarkdown(msg.Chat.ID, msgs.MultipleStationsFound(text, names))
			return
		}
		s := results[0]
		st.Data["dest_name"] = s.Name
		st.Data["dest_code"] = s.ID
		st.Step = 3
		_ = b.sessions.Set(ctx, msg.From.ID, st)
		b.sendMarkdown(msg.Chat.ID, msgs.AskAlertTime())

	case 3: // time
		loc := b.loc
		t, err := parseTimeInput(text, loc)
		if err != nil {
			b.sendMarkdown(msg.Chat.ID, msgs.InvalidTime())
			return
		}

		var originLat, originLon float64
		fmt.Sscanf(st.Data["origin_lat"], "%f", &originLat)
		fmt.Sscanf(st.Data["origin_lon"], "%f", &originLon)

		id := uuid.New().String()
		originJSON, _ := json.Marshal(stationInfo{Name: st.Data["origin_name"], Code: st.Data["origin_code"], Lat: originLat, Lon: originLon})
		destJSON, _ := json.Marshal(stationInfo{Name: st.Data["dest_name"], Code: st.Data["dest_code"]})

		alert := &models.OneTimeAlert{
			ID:           id,
			TelegramID:   user.TelegramID,
			Origin:       datatypes.JSON(originJSON),
			Dest:         datatypes.JSON(destJSON),
			ScheduledFor: t,
		}
		payload := AlertPayload{
			ID:         id,
			TelegramID: user.TelegramID,
			OriginName: st.Data["origin_name"],
			OriginCode: st.Data["origin_code"],
			OriginLat:  originLat,
			OriginLon:  originLon,
			DestName:   st.Data["dest_name"],
			DestCode:   st.Data["dest_code"],
			SendTime:   t,
		}

		if err := ScheduleAlert(ctx, b.rc, b.db, alert, payload); err != nil {
			slog.Error("failed to schedule alert", "error", err)
			b.send(msg.Chat.ID, msgs.InternalError())
			return
		}
		_ = b.sessions.Clear(ctx, msg.From.ID)
		timeStr := t.In(loc).Format("02 Jan 2006 15:04")
		b.sendMarkdown(msg.Chat.ID, msgs.AlertSet(st.Data["origin_name"], st.Data["dest_name"], timeStr, id))
	}
}

// ---- Schedule + Weather Rendering ----

type stationInfo struct {
	Name string  `json:"name"`
	Code string  `json:"code"`
	Lat  float64 `json:"lat,omitempty"`
	Lon  float64 `json:"lon,omitempty"`
}

func (b *Bot) sendScheduleWithWeather(ctx context.Context, chatID int64, origin, dest *stationInfo, t time.Time, msgs *i18n.Messages) {
	b.sendScheduleWithWeatherRaw(ctx, chatID, origin, dest, t, msgs)
}

func (b *Bot) sendScheduleWithWeatherRaw(ctx context.Context, chatID int64, origin, dest *stationInfo, t time.Time, msgs *i18n.Messages) {
	var sb strings.Builder

	// Weather block.
	if origin.Lat != 0 && origin.Lon != 0 {
		f, err := b.weather.Get(ctx, origin.Lat, origin.Lon)
		if err != nil {
			slog.Warn("weather fetch failed", "error", err)
		} else {
			sb.WriteString(fmt.Sprintf("%s *Weather @ %s*: %.0f°C %s, feels %.0f°C, %.0f%% rain\n\n",
				f.Emoji(), escapeMarkdown(origin.Name), f.Temp, f.Condition, f.FeelsLike, f.Precip))
		}
	}

	// Schedule block.
	loc := b.loc
	windowStart := t.Add(-1 * time.Hour)
	windowEnd := t.Add(1 * time.Hour)
	window := fmt.Sprintf("%s–%s", windowStart.In(loc).Format("15:04"), windowEnd.In(loc).Format("15:04"))

	schedules, err := b.getSchedulesBetween(ctx, origin.Code, dest.Code, windowStart, windowEnd)
	if err != nil {
		slog.Warn("schedule fetch failed", "error", err)
		b.sendMarkdown(chatID, sb.String()+msgs.InternalError())
		return
	}

	if len(schedules) == 0 {
		tripSummary, err := b.buildTripPlanSummary(ctx, origin, dest, t)
		if err != nil {
			slog.Warn("trip plan fallback failed", "error", err)
		}
		if strings.TrimSpace(tripSummary) != "" {
			b.sendMarkdown(chatID, sb.String()+tripSummary)
			return
		}
		b.sendMarkdown(chatID, sb.String()+msgs.NoTrains(origin.Name, dest.Name, window))
		return
	}

	sb.WriteString(fmt.Sprintf("🚆 *Trains %s → %s* (%s):\n", escapeMarkdown(origin.Name), escapeMarkdown(dest.Name), window))
	for _, s := range schedules {
		dep := s.DepartsAt.In(loc).Format("15:04")
		arr := s.ArrivesAt.In(loc).Format("15:04")
		sb.WriteString(fmt.Sprintf("• %s → %s %s\n", dep, arr, escapeMarkdown(s.Line)))
	}
	sb.WriteString("\n_Use /schedule for a full search._")

	b.sendMarkdown(chatID, sb.String())
}

func (b *Bot) buildTripPlanSummary(ctx context.Context, origin, dest *stationInfo, at time.Time) (string, error) {
	if b.db == nil {
		return "", nil
	}

	body := map[string]any{
		"from_id":        strings.ToUpper(strings.TrimSpace(origin.Code)),
		"to_id":          strings.ToUpper(strings.TrimSpace(dest.Code)),
		"at":             at.Format(time.RFC3339),
		"window_minutes": 60,
		"max_results":    3,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	rec := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/trip-plan", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	handler := handlers.NewTripPlanHandler(b.db, nil, b.cfg)
	handler.GetTripPlan(c)
	if rec.Code != http.StatusOK {
		return "", fmt.Errorf("trip plan status %d", rec.Code)
	}

	var resp botTripPlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		return "", err
	}
	if len(resp.Data.Options) == 0 {
		return "", nil
	}

	loc := b.loc
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🚆 *Route options %s → %s*:\n", escapeMarkdown(origin.Name), escapeMarkdown(dest.Name)))
	for i, option := range resp.Data.Options {
		if i >= 3 {
			break
		}
		legs := option.Legs
		if len(legs) == 0 {
			continue
		}
		departAt, departErr := time.Parse(time.RFC3339, option.DepartAt)
		arriveAt, arriveErr := time.Parse(time.RFC3339, option.ArriveAt)
		if departErr != nil || arriveErr != nil {
			continue
		}
		if len(legs) == 1 {
			sb.WriteString(fmt.Sprintf("• %s → %s %s\n",
				departAt.In(loc).Format("15:04"),
				arriveAt.In(loc).Format("15:04"),
				escapeMarkdown(legs[0].TrainID)))
			continue
		}

		first := legs[0]
		last := legs[len(legs)-1]
		transferAt := ""
		if len(legs) > 1 {
			transferAt = b.stationNameOrCode(ctx, legs[0].To)
		}
		sb.WriteString(fmt.Sprintf("• %s → %s %s → %s via %s\n",
			departAt.In(loc).Format("15:04"),
			arriveAt.In(loc).Format("15:04"),
			escapeMarkdown(first.TrainID),
			escapeMarkdown(last.TrainID),
			escapeMarkdown(transferAt)))
	}

	if sb.Len() == 0 {
		return "", nil
	}
	if appURL := b.cfg.AppURL(); appURL != "" {
		sb.WriteString("\n_Open /app for the full planner._")
	}
	return sb.String(), nil
}

// ---- DB Helpers ----

func (b *Bot) getOrCreateUser(ctx context.Context, telegramID int64, firstName string) (*models.BotUser, error) {
	var user models.BotUser
	err := b.db.WithContext(ctx).First(&user, "telegram_id = ?", telegramID).Error
	if err == gorm.ErrRecordNotFound {
		defaultDays, _ := json.Marshal([]string{"mon", "tue", "wed", "thu", "fri"})
		user = models.BotUser{
			TelegramID: telegramID,
			Lang:       "en",
			WorkDays:   datatypes.JSON(defaultDays),
		}
		if err := b.db.WithContext(ctx).Create(&user).Error; err != nil {
			return nil, err
		}
		return &user, nil
	}
	return &user, err
}

func (b *Bot) getAllKRLStations(ctx context.Context) ([]models.Station, error) {
	var stations []models.Station
	err := b.db.WithContext(ctx).Where("type = ?", "KRL").Find(&stations).Error
	return stations, err
}

func (b *Bot) getSchedulesBetween(ctx context.Context, originCode, destCode string, from, to time.Time) ([]models.Schedule, error) {
	var schedules []models.Schedule
	err := b.db.WithContext(ctx).
		Where("station_id = ? AND destination_id = ? AND departs_at >= ? AND departs_at <= ?",
			originCode, destCode, from, to).
		Order("departs_at asc").
		Limit(10).
		Find(&schedules).Error
	return schedules, err
}

func (b *Bot) stationNameOrCode(ctx context.Context, code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if b.db != nil {
		var station models.Station
		if err := b.db.WithContext(ctx).Select("name").First(&station, "id = ?", code).Error; err == nil && strings.TrimSpace(station.Name) != "" {
			return station.Name
		}
	}
	return code
}

// ---- Telegram Send Helpers ----

func (b *Bot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.client.Send(msg); err != nil {
		slog.Error("telegram send error", "error", err)
	}
}

func (b *Bot) sendMarkdown(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	if _, err := b.client.Send(msg); err != nil {
		slog.Error("telegram send markdown error", "error", err, "text_len", len(text))
	}
}

func (b *Bot) sendMarkdownWithMarkup(chatID int64, text string, markup interface{}) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	switch typed := markup.(type) {
	case tgbotapi.ReplyKeyboardMarkup:
		msg.ReplyMarkup = typed
	case tgbotapi.InlineKeyboardMarkup:
		msg.ReplyMarkup = typed
	}
	if _, err := b.client.Send(msg); err != nil {
		slog.Error("telegram send with markup error", "error", err, "text_len", len(text))
	}
}

func (b *Bot) editOrSendMarkdown(chatID int64, callback *tgbotapi.CallbackQuery, text string, markup tgbotapi.InlineKeyboardMarkup) {
	if callback == nil || callback.Message == nil {
		b.sendMarkdownWithMarkup(chatID, text, markup)
		return
	}

	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, callback.Message.MessageID, text, markup)
	edit.ParseMode = tgbotapi.ModeMarkdown
	if _, err := b.client.Request(edit); err == nil {
		return
	}
	b.sendMarkdownWithMarkup(chatID, text, markup)
}

// SendMessage is a public helper for the alert dispatcher to deliver messages.
func (b *Bot) SendMessage(chatID int64, text string) {
	b.sendMarkdown(chatID, text)
}

// ---- Utility Functions ----

func (b *Bot) stationFromJSON(raw datatypes.JSON) *stationInfo {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s stationInfo
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	if s.Name == "" {
		return nil
	}
	return &s
}

func latFromStation(s models.Station) float64 {
	var meta map[string]interface{}
	if err := json.Unmarshal(s.Metadata, &meta); err == nil {
		if lat, ok := meta["lat"].(float64); ok {
			return lat
		}
	}
	return 0
}

func lonFromStation(s models.Station) float64 {
	var meta map[string]interface{}
	if err := json.Unmarshal(s.Metadata, &meta); err == nil {
		if lon, ok := meta["lon"].(float64); ok {
			return lon
		}
	}
	return 0
}

func isValidTime(s string) bool {
	_, err := time.Parse("15:04", s)
	return err == nil
}

// parseTimeInput parses "HH:MM", "now", "today HH:MM", "tomorrow HH:MM".
func parseTimeInput(s string, loc *time.Location) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	now := time.Now().In(loc)

	if s == "now" {
		return now, nil
	}

	if strings.HasPrefix(s, "tomorrow ") {
		parts := strings.SplitN(s, " ", 2)
		t, err := time.ParseInLocation("15:04", parts[1], loc)
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(now.Year(), now.Month(), now.Day()+1, t.Hour(), t.Minute(), 0, 0, loc), nil
	}

	if strings.HasPrefix(s, "today ") {
		parts := strings.SplitN(s, " ", 2)
		t, err := time.ParseInLocation("15:04", parts[1], loc)
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc), nil
	}

	t, err := time.ParseInLocation("15:04", s, loc)
	if err != nil {
		return time.Time{}, err
	}
	result := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	if result.Before(now) {
		result = result.Add(24 * time.Hour)
	}
	return result, nil
}

func parseWorkDays(s string) []string {
	valid := map[string]bool{"mon": true, "tue": true, "wed": true, "thu": true, "fri": true, "sat": true, "sun": true}
	parts := strings.Split(strings.ToLower(s), ",")
	var days []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if valid[p] {
			days = append(days, p)
		}
	}
	return days
}

// escapeMarkdown escapes special Markdown characters for Telegram.
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"`", "\\`",
	)
	return replacer.Replace(s)
}
