package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/comu/api/internal/bot/i18n"
	"github.com/comu/api/internal/bot/session"
	"github.com/comu/api/internal/config"
	"github.com/comu/api/internal/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCallbackDataRoundTrip(t *testing.T) {
	payload := callbackPayload{
		Version: 3,
		Action:  callbackActionNavigate,
		Target:  menuScreenPlanTrip,
		Value:   "home",
	}

	encoded := payload.Encode()
	decoded, err := parseCallbackPayload(encoded)
	if err != nil {
		t.Fatalf("parseCallbackPayload() error = %v", err)
	}

	if decoded != payload {
		t.Fatalf("decoded payload = %#v, want %#v", decoded, payload)
	}
}

func TestParseCallbackPayloadRejectsInvalidFormat(t *testing.T) {
	_, err := parseCallbackPayload("broken")
	if err == nil {
		t.Fatal("expected invalid callback payload to fail")
	}
}

func TestMainMenuKeyboardContainsPrimaryActions(t *testing.T) {
	msgs := i18n.New("en")
	keyboard := buildMainMenuKeyboard(msgs)

	if len(keyboard.Keyboard) < 3 {
		t.Fatalf("expected at least 3 keyboard rows, got %d", len(keyboard.Keyboard))
	}

	got := []string{
		keyboard.Keyboard[0][0].Text,
		keyboard.Keyboard[0][1].Text,
		keyboard.Keyboard[1][0].Text,
		keyboard.Keyboard[1][1].Text,
		keyboard.Keyboard[2][0].Text,
		keyboard.Keyboard[2][1].Text,
	}
	want := []string{
		msgs.MenuMorning(),
		msgs.MenuEvening(),
		msgs.MenuPlanTrip(),
		msgs.MenuPreferredStations(),
		msgs.MenuAlerts(),
		msgs.MenuSettings(),
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("keyboard button %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPlanTripKeyboardIncludesAppButton(t *testing.T) {
	msgs := i18n.New("en")
	keyboard := buildPlanTripKeyboard(msgs, "https://example.com/app", 5)

	if len(keyboard.InlineKeyboard) == 0 {
		t.Fatal("expected inline keyboard rows")
	}

	foundURL := false
	for _, row := range keyboard.InlineKeyboard {
		for _, button := range row {
			if button.Text == msgs.MenuOpenApp() && button.URL != nil && *button.URL == "https://example.com/app" {
				foundURL = true
			}
		}
	}
	if !foundURL {
		t.Fatal("expected plan trip keyboard to include app URL button")
	}
}

func TestHandleCallbackActionRejectsStaleVersion(t *testing.T) {
	client := &fakeTelegramClient{}
	b := &Bot{client: client}
	msgs := i18n.New("en")

	callback := &tgbotapi.CallbackQuery{
		ID: "cb-1",
		Message: &tgbotapi.Message{
			MessageID: 10,
			Chat:      &tgbotapi.Chat{ID: 99},
		},
		Data: callbackPayload{
			Version: 1,
			Action:  callbackActionNavigate,
			Target:  menuScreenSettings,
		}.Encode(),
	}

	state := &session.MenuState{Version: 2, Screen: string(menuScreenMain)}
	b.handleCallbackAction(callback, nil, msgs, state)

	if len(client.requests) == 0 {
		t.Fatal("expected callback answer request")
	}

	answer, ok := client.requests[0].(tgbotapi.CallbackConfig)
	if !ok {
		t.Fatalf("expected first request to be CallbackConfig, got %T", client.requests[0])
	}
	if !strings.Contains(answer.Text, "expired") {
		t.Fatalf("expected stale callback notice, got %q", answer.Text)
	}
}

func TestShowMainMenuSendsReplyKeyboard(t *testing.T) {
	client := &fakeTelegramClient{}
	b := &Bot{
		client: client,
		cfg:    &config.Config{},
		loc:    time.FixedZone("WIB", 7*60*60),
	}

	b.showMainMenu(context.Background(), 77, i18n.New("en"), "hello")

	if len(client.sent) != 1 {
		t.Fatalf("expected one sent message, got %d", len(client.sent))
	}

	msg, ok := client.sent[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", client.sent[0])
	}
	keyboard, ok := msg.ReplyMarkup.(tgbotapi.ReplyKeyboardMarkup)
	if !ok {
		t.Fatalf("expected reply keyboard markup, got %T", msg.ReplyMarkup)
	}
	if keyboard.Keyboard[0][0].Text != "Morning" {
		t.Fatalf("expected Morning button, got %q", keyboard.Keyboard[0][0].Text)
	}
}

func TestHandleMenuTextPlanTripShowsInlineMenu(t *testing.T) {
	client := &fakeTelegramClient{}
	user := testBotUser(t)
	b := &Bot{
		client: client,
		cfg:    &config.Config{AppBaseURL: "https://comu.example.com"},
		loc:    time.FixedZone("WIB", 7*60*60),
	}
	msgs := i18n.New("en")
	msg := &tgbotapi.Message{
		Text: msgs.MenuPlanTrip(),
		Chat: &tgbotapi.Chat{ID: 55},
		From: &tgbotapi.User{ID: 12},
	}

	if handled := b.handleMenuText(context.Background(), msg, user, msgs); !handled {
		t.Fatal("expected plan trip button text to be handled")
	}
	if len(client.sent) != 1 {
		t.Fatalf("expected one sent message, got %d", len(client.sent))
	}

	sent, ok := client.sent[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", client.sent[0])
	}
	markup, ok := sent.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("expected inline keyboard markup, got %T", sent.ReplyMarkup)
	}

	foundAppURL := false
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.URL != nil && *button.URL == "https://comu.example.com/app" {
				foundAppURL = true
			}
		}
	}
	if !foundAppURL {
		t.Fatal("expected plan trip menu to contain normalized /app URL")
	}
}

func TestSendScheduleWithWeatherRawFallsBackToTripPlan(t *testing.T) {
	db := setupBotTestDB(t)
	seedTripPlannerRoute(t, db)

	client := &fakeTelegramClient{}
	b := &Bot{
		client: client,
		db:     db,
		cfg:    &config.Config{},
		loc:    mustJakartaLocation(t),
	}
	msgs := i18n.New("en")
	at := mustJakartaDateTime(t, "2026-03-05 15:51")

	b.sendScheduleWithWeatherRaw(
		context.Background(),
		88,
		&stationInfo{Name: "Rawa Buaya", Code: "RW"},
		&stationInfo{Name: "Sudirman Baru", Code: "SUDB"},
		at,
		msgs,
	)

	if len(client.sent) != 1 {
		t.Fatalf("expected one sent message, got %d", len(client.sent))
	}
	msg, ok := client.sent[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", client.sent[0])
	}
	if strings.Contains(msg.Text, "No trains found") {
		t.Fatalf("expected planner fallback instead of no-trains text, got %q", msg.Text)
	}
	for _, want := range []string{
		"*Option 1*",
		"1 Transit",
		"Duration 73 min",
		"A1 | Commuter Line Tangerang | Rawa Buaya → Duri",
		"Rawa Buaya dep 15:52 • Duri arr 16:08",
		"Transit at Duri • arrive 16:08 • depart 16:17 • wait 9 min",
		"C1 | Commuter Line Cikarang | Duri → Sudirman Baru",
		"Duri dep 16:17 • Sudirman Baru arr 17:05",
	} {
		if !strings.Contains(msg.Text, want) {
			t.Fatalf("expected detailed trip plan output to contain %q, got %q", want, msg.Text)
		}
	}
}

type fakeTelegramClient struct {
	sent     []tgbotapi.Chattable
	requests []tgbotapi.Chattable
}

func (f *fakeTelegramClient) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	f.sent = append(f.sent, c)
	return tgbotapi.Message{}, nil
}

func (f *fakeTelegramClient) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	f.requests = append(f.requests, c)
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func testBotUser(t *testing.T) *models.BotUser {
	t.Helper()
	homeJSON, err := json.Marshal(stationInfo{Name: "Depok", Code: "DP"})
	if err != nil {
		t.Fatalf("marshal home: %v", err)
	}
	awayJSON, err := json.Marshal(stationInfo{Name: "Jakarta Kota", Code: "JAKK"})
	if err != nil {
		t.Fatalf("marshal away: %v", err)
	}
	return &models.BotUser{
		TelegramID:  12,
		HomeStation: datatypes.JSON(homeJSON),
		AwayStation: datatypes.JSON(awayJSON),
		Lang:        "en",
	}
}

func setupBotTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Station{}, &models.Schedule{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func seedTripPlannerRoute(t *testing.T, db *gorm.DB) {
	t.Helper()
	stations := []models.Station{
		{UID: "rawa-buaya", ID: "RW", Name: "Rawa Buaya", Type: "KRL", Metadata: datatypes.JSON(`{}`)},
		{UID: "duri", ID: "DU", Name: "Duri", Type: "KRL", Metadata: datatypes.JSON(`{}`)},
		{UID: "sudirman-baru", ID: "SUDB", Name: "Sudirman Baru", Type: "KRL", Metadata: datatypes.JSON(`{}`)},
	}
	for _, station := range stations {
		if err := db.Create(&station).Error; err != nil {
			t.Fatalf("seed station %s: %v", station.ID, err)
		}
	}

	schedules := []models.Schedule{
		{ID: "a1-rw", TrainID: "A1", Line: "Commuter Line Tangerang", Route: "RW-DU", OriginID: "RW", DestinationID: "DU", StationID: "RW", DepartsAt: mustJakartaDateTime(t, "2026-03-05 15:52"), ArrivesAt: mustJakartaDateTime(t, "2026-03-05 15:52"), Metadata: datatypes.JSON(`{}`)},
		{ID: "a1-du", TrainID: "A1", Line: "Commuter Line Tangerang", Route: "RW-DU", OriginID: "RW", DestinationID: "DU", StationID: "DU", DepartsAt: mustJakartaDateTime(t, "2026-03-05 16:08"), ArrivesAt: mustJakartaDateTime(t, "2026-03-05 16:08"), Metadata: datatypes.JSON(`{}`)},
		{ID: "c1-du", TrainID: "C1", Line: "Commuter Line Cikarang", Route: "DU-SUDB", OriginID: "DU", DestinationID: "SUDB", StationID: "DU", DepartsAt: mustJakartaDateTime(t, "2026-03-05 16:17"), ArrivesAt: mustJakartaDateTime(t, "2026-03-05 16:17"), Metadata: datatypes.JSON(`{}`)},
		{ID: "c1-sudb", TrainID: "C1", Line: "Commuter Line Cikarang", Route: "DU-SUDB", OriginID: "DU", DestinationID: "SUDB", StationID: "SUDB", DepartsAt: mustJakartaDateTime(t, "2026-03-05 17:05"), ArrivesAt: mustJakartaDateTime(t, "2026-03-05 17:05"), Metadata: datatypes.JSON(`{}`)},
	}
	for _, schedule := range schedules {
		if err := db.Create(&schedule).Error; err != nil {
			t.Fatalf("seed schedule %s: %v", schedule.ID, err)
		}
	}
}

func mustJakartaDateTime(t *testing.T, value string) time.Time {
	t.Helper()
	loc := mustJakartaLocation(t)
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatalf("parse time %s: %v", value, err)
	}
	return parsed
}

func mustJakartaLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load jakarta loc: %v", err)
	}
	return loc
}
