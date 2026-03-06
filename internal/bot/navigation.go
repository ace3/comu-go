package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/comu/api/internal/bot/i18n"
	"github.com/comu/api/internal/bot/session"
	"github.com/comu/api/internal/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type callbackAction string
type menuScreen string

const (
	callbackActionNavigate  callbackAction = "nav"
	callbackActionCommute   callbackAction = "trip"
	callbackActionSettings  callbackAction = "set"
	callbackActionAlerts    callbackAction = "alert"
	callbackActionPlan      callbackAction = "plan"
	callbackActionPreferred callbackAction = "pref"

	menuScreenMain      menuScreen = "main"
	menuScreenPlanTrip  menuScreen = "plan"
	menuScreenPlanDest  menuScreen = "plan_dest"
	menuScreenSettings  menuScreen = "settings"
	menuScreenAlerts    menuScreen = "alerts"
	menuScreenPreferred menuScreen = "preferred"
	menuScreenCommute   menuScreen = "commute"
)

type callbackPayload struct {
	Version int
	Action  callbackAction
	Target  menuScreen
	Value   string
}

func (p callbackPayload) Encode() string {
	parts := []string{"v1", strconv.Itoa(p.Version), string(p.Action), string(p.Target), p.Value}
	return strings.Join(parts, "|")
}

func parseCallbackPayload(raw string) (callbackPayload, error) {
	parts := strings.Split(raw, "|")
	if len(parts) < 4 || parts[0] != "v1" {
		return callbackPayload{}, errors.New("invalid payload")
	}
	version, err := strconv.Atoi(parts[1])
	if err != nil {
		return callbackPayload{}, fmt.Errorf("invalid version: %w", err)
	}
	value := ""
	if len(parts) > 4 {
		value = parts[4]
	}
	return callbackPayload{
		Version: version,
		Action:  callbackAction(parts[2]),
		Target:  menuScreen(parts[3]),
		Value:   value,
	}, nil
}

func buildMainMenuKeyboard(msgs *i18n.Messages) tgbotapi.ReplyKeyboardMarkup {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(msgs.MenuMorning()),
			tgbotapi.NewKeyboardButton(msgs.MenuEvening()),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(msgs.MenuPlanTrip()),
			tgbotapi.NewKeyboardButton(msgs.MenuPreferredStations()),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(msgs.MenuAlerts()),
			tgbotapi.NewKeyboardButton(msgs.MenuSettings()),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(msgs.MenuHelp()),
		),
	)
	keyboard.ResizeKeyboard = true
	keyboard.Selective = true
	keyboard.InputFieldPlaceholder = msgs.MenuNavigationHint()
	return keyboard
}

func buildPlanTripKeyboard(msgs *i18n.Messages, appURL string, version int) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuMorning(), callbackPayload{Version: version, Action: callbackActionPlan, Target: menuScreenPlanTrip, Value: "home"}.Encode()),
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuEvening(), callbackPayload{Version: version, Action: callbackActionPlan, Target: menuScreenPlanTrip, Value: "away"}.Encode()),
		),
	}
	if appURL != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(msgs.MenuOpenApp(), appURL),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(msgs.MenuBack(), callbackPayload{Version: version, Action: callbackActionNavigate, Target: menuScreenMain}.Encode()),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func buildSettingsKeyboard(msgs *i18n.Messages, version int) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuSetRoute(), callbackPayload{Version: version, Action: callbackActionSettings, Target: menuScreenSettings, Value: "route"}.Encode()),
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuWorkDays(), callbackPayload{Version: version, Action: callbackActionSettings, Target: menuScreenSettings, Value: "days"}.Encode()),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuToggleNotifications(), callbackPayload{Version: version, Action: callbackActionSettings, Target: menuScreenSettings, Value: "notifs"}.Encode()),
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuLanguage(), callbackPayload{Version: version, Action: callbackActionSettings, Target: menuScreenSettings, Value: "lang"}.Encode()),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuBack(), callbackPayload{Version: version, Action: callbackActionNavigate, Target: menuScreenMain}.Encode()),
		),
	)
}

func buildAlertsKeyboard(msgs *i18n.Messages, version int) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuCreateAlert(), callbackPayload{Version: version, Action: callbackActionAlerts, Target: menuScreenAlerts, Value: "create"}.Encode()),
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuListAlerts(), callbackPayload{Version: version, Action: callbackActionAlerts, Target: menuScreenAlerts, Value: "list"}.Encode()),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuBack(), callbackPayload{Version: version, Action: callbackActionNavigate, Target: menuScreenMain}.Encode()),
		),
	)
}

func buildPreferredKeyboard(msgs *i18n.Messages, version int, user *models.BotUser) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{}
	if home := stationLabel(user, "home"); home != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(home, callbackPayload{Version: version, Action: callbackActionPreferred, Target: menuScreenPreferred, Value: "home"}.Encode()),
		))
	}
	if away := stationLabel(user, "away"); away != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(away, callbackPayload{Version: version, Action: callbackActionPreferred, Target: menuScreenPreferred, Value: "away"}.Encode()),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(msgs.MenuBack(), callbackPayload{Version: version, Action: callbackActionNavigate, Target: menuScreenMain}.Encode()),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func buildPlanDestinationKeyboard(msgs *i18n.Messages, version int, user *models.BotUser, originSlot string, appURL string) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{}
	for _, slot := range []string{"home", "away"} {
		if slot == originSlot {
			continue
		}
		if label := stationLabel(user, slot); label != "" {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, callbackPayload{Version: version, Action: callbackActionPlan, Target: menuScreenPlanDest, Value: slot}.Encode()),
			))
		}
	}
	if appURL != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(msgs.MenuOpenApp(), appURL),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(msgs.MenuBack(), callbackPayload{Version: version, Action: callbackActionNavigate, Target: menuScreenPlanTrip}.Encode()),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func buildCommuteKeyboard(msgs *i18n.Messages, version int, appURL string) tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuHomeToAway(), callbackPayload{Version: version, Action: callbackActionCommute, Target: menuScreenCommute, Value: "morning"}.Encode()),
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuAwayToHome(), callbackPayload{Version: version, Action: callbackActionCommute, Target: menuScreenCommute, Value: "evening"}.Encode()),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuRefresh(), callbackPayload{Version: version, Action: callbackActionCommute, Target: menuScreenCommute, Value: "refresh"}.Encode()),
			tgbotapi.NewInlineKeyboardButtonData(msgs.MenuBack(), callbackPayload{Version: version, Action: callbackActionNavigate, Target: menuScreenMain}.Encode()),
		),
	}
	if appURL != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(msgs.MenuOpenApp(), appURL),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func stationLabel(user *models.BotUser, slot string) string {
	if user == nil {
		return ""
	}
	var raw *stationInfo
	if slot == "away" {
		raw = stationFromBotUser(user.AwayStation)
	} else {
		raw = stationFromBotUser(user.HomeStation)
	}
	if raw == nil {
		return ""
	}
	return fmt.Sprintf("%s (%s)", raw.Name, raw.Code)
}

func stationFromBotUser(raw []byte) *stationInfo {
	var s stationInfo
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := jsonUnmarshal(raw, &s); err != nil {
		return nil
	}
	if s.Name == "" {
		return nil
	}
	return &s
}

var jsonUnmarshal = func(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (b *Bot) showMainMenu(ctx context.Context, chatID int64, msgs *i18n.Messages, text string) {
	version := b.bumpMenu(ctx, chatID, menuScreenMain, nil)
	_ = version
	b.sendMarkdownWithMarkup(chatID, text, buildMainMenuKeyboard(msgs))
}

func (b *Bot) showSettingsMenu(ctx context.Context, chatID int64, userID int64, msgs *i18n.Messages, edit *tgbotapi.CallbackQuery) {
	version := b.bumpMenu(ctx, userID, menuScreenSettings, nil)
	markup := buildSettingsKeyboard(msgs, version)
	b.editOrSendMarkdown(chatID, edit, msgs.SettingsPrompt(), markup)
}

func (b *Bot) showAlertsMenu(ctx context.Context, chatID int64, userID int64, msgs *i18n.Messages, edit *tgbotapi.CallbackQuery) {
	version := b.bumpMenu(ctx, userID, menuScreenAlerts, nil)
	markup := buildAlertsKeyboard(msgs, version)
	b.editOrSendMarkdown(chatID, edit, msgs.AlertsPrompt(), markup)
}

func (b *Bot) showPreferredMenu(ctx context.Context, chatID int64, user *models.BotUser, msgs *i18n.Messages, edit *tgbotapi.CallbackQuery) {
	version := b.bumpMenu(ctx, user.TelegramID, menuScreenPreferred, nil)
	markup := buildPreferredKeyboard(msgs, version, user)
	b.editOrSendMarkdown(chatID, edit, msgs.PreferredStationsPrompt(), markup)
}

func (b *Bot) showPlanTripMenu(ctx context.Context, chatID int64, userID int64, msgs *i18n.Messages, edit *tgbotapi.CallbackQuery) {
	version := b.bumpMenu(ctx, userID, menuScreenPlanTrip, nil)
	markup := buildPlanTripKeyboard(msgs, b.cfg.AppURL(), version)
	text := msgs.PlanTripPrompt()
	if b.cfg.AppURL() == "" {
		text = text + "\n\n" + msgs.AppLinkUnavailable()
	}
	b.editOrSendMarkdown(chatID, edit, text, markup)
}

func (b *Bot) showCommuteActions(ctx context.Context, chatID int64, userID int64, last string, msgs *i18n.Messages) {
	data := map[string]string{"last": last}
	version := b.bumpMenu(ctx, userID, menuScreenCommute, data)
	b.sendMarkdownWithMarkup(chatID, msgs.CommuteActionsPrompt(), buildCommuteKeyboard(msgs, version, b.cfg.AppURL()))
}

func (b *Bot) bumpMenu(ctx context.Context, userID int64, screen menuScreen, data map[string]string) int {
	if b.sessions == nil {
		return 1
	}
	current, err := b.sessions.GetMenu(ctx, userID)
	if err != nil {
		slog.Warn("menu state load failed", "error", err)
	}
	version := 1
	if current != nil {
		version = current.Version + 1
	}
	if err := b.sessions.SetMenu(ctx, userID, &session.MenuState{Screen: string(screen), Version: version, Data: data}); err != nil {
		slog.Warn("menu state save failed", "error", err)
	}
	return version
}

func (b *Bot) handleMenuText(ctx context.Context, msg *tgbotapi.Message, user *models.BotUser, msgs *i18n.Messages) bool {
	switch strings.TrimSpace(msg.Text) {
	case msgs.MenuMorning():
		b.handleGoMorning(ctx, msg, user, msgs)
		return true
	case msgs.MenuEvening():
		b.handleGoEvening(ctx, msg, user, msgs)
		return true
	case msgs.MenuPlanTrip():
		b.showPlanTripMenu(ctx, msg.Chat.ID, user.TelegramID, msgs, nil)
		return true
	case msgs.MenuPreferredStations():
		b.showPreferredMenu(ctx, msg.Chat.ID, user, msgs, nil)
		return true
	case msgs.MenuAlerts():
		b.showAlertsMenu(ctx, msg.Chat.ID, user.TelegramID, msgs, nil)
		return true
	case msgs.MenuSettings():
		b.showSettingsMenu(ctx, msg.Chat.ID, user.TelegramID, msgs, nil)
		return true
	case msgs.MenuHelp():
		b.showMainMenu(ctx, msg.Chat.ID, msgs, msgs.Help())
		return true
	default:
		return false
	}
}

func (b *Bot) handleCallbackAction(callback *tgbotapi.CallbackQuery, user *models.BotUser, msgs *i18n.Messages, current *session.MenuState) {
	payload, err := parseCallbackPayload(callback.Data)
	if err != nil {
		b.answerCallback(callback, msgs.ButtonActionExpired())
		return
	}
	if current != nil && payload.Version != current.Version {
		b.answerCallback(callback, msgs.ButtonActionExpired())
		return
	}
	b.answerCallback(callback, "")
}

func (b *Bot) handleCallbackQuery(ctx context.Context, callback *tgbotapi.CallbackQuery, user *models.BotUser, msgs *i18n.Messages) {
	var current *session.MenuState
	if b.sessions != nil {
		var err error
		current, err = b.sessions.GetMenu(ctx, user.TelegramID)
		if err != nil {
			slog.Warn("menu state fetch failed", "error", err)
		}
	}
	payload, parseErr := parseCallbackPayload(callback.Data)
	if parseErr != nil {
		b.answerCallback(callback, msgs.ButtonActionExpired())
		return
	}
	if current != nil && payload.Version != current.Version {
		b.answerCallback(callback, msgs.ButtonActionExpired())
		return
	}
	switch payload.Action {
	case callbackActionNavigate:
		b.answerCallback(callback, "")
		switch payload.Target {
		case menuScreenMain:
			b.showMainMenu(ctx, callback.Message.Chat.ID, msgs, msgs.MenuNavigationHint())
		case menuScreenSettings:
			b.showSettingsMenu(ctx, callback.Message.Chat.ID, user.TelegramID, msgs, callback)
		case menuScreenAlerts:
			b.showAlertsMenu(ctx, callback.Message.Chat.ID, user.TelegramID, msgs, callback)
		case menuScreenPreferred:
			b.showPreferredMenu(ctx, callback.Message.Chat.ID, user, msgs, callback)
		case menuScreenPlanTrip:
			b.showPlanTripMenu(ctx, callback.Message.Chat.ID, user.TelegramID, msgs, callback)
		}
	case callbackActionSettings:
		b.answerCallback(callback, "")
		b.handleSettingsCallback(ctx, callback, user, msgs, payload.Value)
	case callbackActionAlerts:
		b.answerCallback(callback, "")
		b.handleAlertsCallback(ctx, callback, user, msgs, payload.Value)
	case callbackActionPreferred:
		b.answerCallback(callback, "")
		b.handlePreferredCallback(ctx, callback, user, msgs, payload.Value)
	case callbackActionPlan:
		b.answerCallback(callback, "")
		b.handlePlanCallback(ctx, callback, user, msgs, current, payload.Value)
	case callbackActionCommute:
		b.answerCallback(callback, "")
		b.handleCommuteCallback(ctx, callback, user, msgs, current, payload.Value)
	}
}

func (b *Bot) handleSettingsCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, user *models.BotUser, msgs *i18n.Messages, value string) {
	msg := callback.Message
	switch value {
	case "route":
		b.startSetRouteForUser(ctx, user.TelegramID, msg.Chat.ID, msgs)
	case "days":
		b.startSetScheduleForUser(ctx, user.TelegramID, msg.Chat.ID, user, msgs)
	case "notifs":
		b.handleToggleNotifs(ctx, msg, user, msgs)
	case "lang":
		b.handleLang(ctx, msg, user, msgs)
	}
}

func (b *Bot) handleAlertsCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, user *models.BotUser, msgs *i18n.Messages, value string) {
	msg := callback.Message
	switch value {
	case "create":
		b.startScheduleOnceForUser(ctx, user.TelegramID, msg.Chat.ID, msgs)
	case "list":
		b.handleListAlerts(ctx, msg, user, msgs)
	}
}

func (b *Bot) handlePreferredCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, user *models.BotUser, msgs *i18n.Messages, value string) {
	b.prefillScheduleOrigin(ctx, user.TelegramID, callback.Message.Chat.ID, user, msgs, value)
}

func (b *Bot) handlePlanCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, user *models.BotUser, msgs *i18n.Messages, current *session.MenuState, value string) {
	if current != nil && current.Screen == string(menuScreenPlanDest) {
		b.prefillScheduleDestination(ctx, user.TelegramID, callback.Message.Chat.ID, user, msgs, current, value)
		return
	}

	version := b.bumpMenu(ctx, user.TelegramID, menuScreenPlanDest, map[string]string{"origin_slot": value})
	text := msgs.AskScheduleDest()
	b.editOrSendMarkdown(callback.Message.Chat.ID, callback, text, buildPlanDestinationKeyboard(msgs, version, user, value, b.cfg.AppURL()))
}

func (b *Bot) handleCommuteCallback(ctx context.Context, callback *tgbotapi.CallbackQuery, user *models.BotUser, msgs *i18n.Messages, current *session.MenuState, value string) {
	msg := callback.Message
	last := value
	if value == "refresh" && current != nil {
		last = current.Data["last"]
		if last == "" {
			last = "morning"
		}
	}
	if last == "evening" {
		b.handleGoEvening(ctx, msg, user, msgs)
		return
	}
	b.handleGoMorning(ctx, msg, user, msgs)
}

func (b *Bot) prefillScheduleOrigin(ctx context.Context, userID, chatID int64, user *models.BotUser, msgs *i18n.Messages, slot string) {
	station := b.userStationBySlot(user, slot)
	if station == nil {
		b.sendMarkdown(chatID, msgs.NoRouteSet())
		return
	}
	if b.sessions != nil {
		_ = b.sessions.Set(ctx, userID, &session.State{
			Command: "schedule",
			Step:    2,
			Data: map[string]string{
				"origin_name": station.Name,
				"origin_code": station.Code,
				"origin_lat":  fmt.Sprintf("%f", station.Lat),
				"origin_lon":  fmt.Sprintf("%f", station.Lon),
			},
		})
	}
	b.sendMarkdown(chatID, msgs.AskScheduleDest())
}

func (b *Bot) prefillScheduleDestination(ctx context.Context, userID, chatID int64, user *models.BotUser, msgs *i18n.Messages, current *session.MenuState, slot string) {
	originSlot := ""
	if current != nil {
		originSlot = current.Data["origin_slot"]
	}
	origin := b.userStationBySlot(user, originSlot)
	dest := b.userStationBySlot(user, slot)
	if origin == nil || dest == nil || origin.Code == dest.Code {
		b.sendMarkdown(chatID, msgs.NoRouteSet())
		return
	}
	if b.sessions != nil {
		_ = b.sessions.Set(ctx, userID, &session.State{
			Command: "schedule",
			Step:    3,
			Data: map[string]string{
				"origin_name": origin.Name,
				"origin_code": origin.Code,
				"origin_lat":  fmt.Sprintf("%f", origin.Lat),
				"origin_lon":  fmt.Sprintf("%f", origin.Lon),
				"dest_name":   dest.Name,
				"dest_code":   dest.Code,
			},
		})
	}
	b.sendMarkdown(chatID, msgs.AskScheduleTime())
}

func (b *Bot) userStationBySlot(user *models.BotUser, slot string) *stationInfo {
	if slot == "away" {
		return b.stationFromJSON(user.AwayStation)
	}
	return b.stationFromJSON(user.HomeStation)
}

func (b *Bot) answerCallback(callback *tgbotapi.CallbackQuery, text string) {
	if b.client == nil || callback == nil {
		return
	}
	cfg := tgbotapi.NewCallback(callback.ID, text)
	if _, err := b.client.Request(cfg); err != nil {
		slog.Warn("callback answer failed", "error", err)
	}
}
