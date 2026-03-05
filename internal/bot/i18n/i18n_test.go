package i18n

import (
	"strings"
	"testing"
)

func TestNew_DefaultsToEN(t *testing.T) {
	m := New("unknown")
	if m.lang != "en" {
		t.Errorf("expected lang=en for unknown input, got %q", m.lang)
	}
}

func TestNew_ID(t *testing.T) {
	m := New("id")
	if m.lang != "id" {
		t.Errorf("expected lang=id, got %q", m.lang)
	}
}

func TestWelcome_EN(t *testing.T) {
	m := New("en")
	msg := m.Welcome("Budi")
	if !strings.Contains(msg, "Budi") {
		t.Error("welcome message should contain first name")
	}
	if !strings.Contains(msg, "/set_route") {
		t.Error("welcome message should contain /set_route hint")
	}
}

func TestWelcome_ID(t *testing.T) {
	m := New("id")
	msg := m.Welcome("Sari")
	if !strings.Contains(msg, "Sari") {
		t.Error("welcome message should contain first name")
	}
}

func TestHelp_ContainsCommands(t *testing.T) {
	for _, lang := range []string{"en", "id"} {
		m := New(lang)
		help := m.Help()
		for _, cmd := range []string{"/go", "/schedule", "/settings"} {
			if !strings.Contains(help, cmd) {
				t.Errorf("[%s] help message missing %q", lang, cmd)
			}
		}
	}
}

func TestRouteSet_ContainsStations(t *testing.T) {
	m := New("en")
	msg := m.RouteSet("Depok", "Jakarta Kota", "07:00", "17:30")
	for _, s := range []string{"Depok", "Jakarta Kota", "07:00", "17:30"} {
		if !strings.Contains(msg, s) {
			t.Errorf("RouteSet message missing %q", s)
		}
	}
}

func TestAlertSet_ContainsID(t *testing.T) {
	m := New("en")
	id := "abc12345-abcd-abcd-abcd-abcdefabcdef"
	msg := m.AlertSet("Depok", "Jakarta Kota", "05 Mar 2026 07:00", id)
	if !strings.Contains(msg, id[:8]) {
		t.Errorf("AlertSet message should contain ID prefix %q", id[:8])
	}
}

func TestWorkScheduleSet_EN(t *testing.T) {
	m := New("en")
	msg := m.WorkScheduleSet([]string{"mon", "tue", "wed"})
	if !strings.Contains(msg, "mon") {
		t.Error("WorkScheduleSet should contain days")
	}
}

func TestLangSwitched(t *testing.T) {
	en := New("en")
	id := New("id")
	enMsg := en.LangSwitched()
	idMsg := id.LangSwitched()
	if enMsg == idMsg {
		t.Error("LangSwitched messages should differ by language")
	}
}

func TestNoRouteSet_BothLangs(t *testing.T) {
	for _, lang := range []string{"en", "id"} {
		m := New(lang)
		msg := m.NoRouteSet()
		if msg == "" {
			t.Errorf("[%s] NoRouteSet returned empty string", lang)
		}
	}
}

func TestSettings_ContainsFields(t *testing.T) {
	m := New("en")
	msg := m.Settings(12345, "Depok", "Jakarta Kota", "07:00", "17:30", []string{"mon", "fri"}, true, "en")
	for _, s := range []string{"12345", "Depok", "07:00", "en"} {
		if !strings.Contains(msg, s) {
			t.Errorf("Settings message missing %q", s)
		}
	}
}
