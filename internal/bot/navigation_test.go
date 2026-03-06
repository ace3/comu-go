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
