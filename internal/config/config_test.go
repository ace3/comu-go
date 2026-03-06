package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Clear env vars
	os.Unsetenv("PORT")
	os.Unsetenv("COMU_ENV")
	os.Unsetenv("SKIPPED_STATIONS")
	os.Unsetenv("ON_DEMAND_SYNC_ENABLED")
	os.Unsetenv("ON_DEMAND_SYNC_MIN_INTERVAL_MINUTES")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.Env != "development" {
		t.Errorf("expected default env development, got %s", cfg.Env)
	}
	if got, want := strings.Join(cfg.SkippedStations, ","), "GGL,CKP,BANDARA,PWK"; got != want {
		t.Errorf("expected default skipped stations %q, got %q", want, got)
	}
	if !cfg.OnDemandSyncEnabled {
		t.Errorf("expected ON_DEMAND_SYNC_ENABLED default true")
	}
	if cfg.OnDemandSyncMinIntervalMinutes != 30 {
		t.Errorf("expected ON_DEMAND_SYNC_MIN_INTERVAL_MINUTES default 30, got %d", cfg.OnDemandSyncMinIntervalMinutes)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("COMU_ENV", "production")
	os.Setenv("DATABASE_URL", "postgres://test")
	os.Setenv("REDIS_URL", "redis://test")
	os.Setenv("KAI_AUTH_TOKEN", "test-token")
	os.Setenv("SYNC_SECRET", "my-secret")
	os.Setenv("APP_BASE_URL", "https://comu.example.com")
	os.Setenv("SKIPPED_STATIONS", "MRI, JAKK ,, BOO")
	os.Setenv("ON_DEMAND_SYNC_ENABLED", "0")
	os.Setenv("ON_DEMAND_SYNC_MIN_INTERVAL_MINUTES", "45")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("COMU_ENV")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("REDIS_URL")
		os.Unsetenv("KAI_AUTH_TOKEN")
		os.Unsetenv("SYNC_SECRET")
		os.Unsetenv("APP_BASE_URL")
		os.Unsetenv("SKIPPED_STATIONS")
		os.Unsetenv("ON_DEMAND_SYNC_ENABLED")
		os.Unsetenv("ON_DEMAND_SYNC_MIN_INTERVAL_MINUTES")
	}()

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Port)
	}
	if cfg.Env != "production" {
		t.Errorf("expected env production, got %s", cfg.Env)
	}
	if cfg.DatabaseURL != "postgres://test" {
		t.Errorf("expected DATABASE_URL postgres://test, got %s", cfg.DatabaseURL)
	}
	if cfg.SyncSecret != "my-secret" {
		t.Errorf("expected SYNC_SECRET my-secret, got %s", cfg.SyncSecret)
	}
	if cfg.AppBaseURL != "https://comu.example.com" {
		t.Errorf("expected APP_BASE_URL to load, got %q", cfg.AppBaseURL)
	}
	if got, want := strings.Join(cfg.SkippedStations, ","), "MRI,JAKK,BOO"; got != want {
		t.Errorf("expected parsed skipped stations %q, got %q", want, got)
	}
	if cfg.OnDemandSyncEnabled {
		t.Errorf("expected ON_DEMAND_SYNC_ENABLED=false, got true")
	}
	if cfg.OnDemandSyncMinIntervalMinutes != 45 {
		t.Errorf("expected ON_DEMAND_SYNC_MIN_INTERVAL_MINUTES 45, got %d", cfg.OnDemandSyncMinIntervalMinutes)
	}
}

func TestAppURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "empty", base: "", want: ""},
		{name: "root base", base: "https://comu.example.com", want: "https://comu.example.com/app"},
		{name: "app path", base: "https://comu.example.com/app", want: "https://comu.example.com/app"},
		{name: "trailing slash", base: "https://comu.example.com/", want: "https://comu.example.com/app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{AppBaseURL: tt.base}
			if got := cfg.AppURL(); got != tt.want {
				t.Fatalf("AppURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidate_AllPresent(t *testing.T) {
	cfg := &Config{
		DatabaseURL:  "postgres://test",
		RedisURL:     "redis://test",
		KAIAuthToken: "token",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidate_AllMissing(t *testing.T) {
	cfg := &Config{}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	for _, v := range []string{"DATABASE_URL", "REDIS_URL", "KAI_AUTH_TOKEN"} {
		if !strings.Contains(errMsg, v) {
			t.Errorf("expected error to mention %s, got: %s", v, errMsg)
		}
	}
}

func TestValidate_PartialMissing(t *testing.T) {
	cfg := &Config{
		DatabaseURL: "postgres://test",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "DATABASE_URL") {
		t.Errorf("DATABASE_URL is set, should not be in error: %s", errMsg)
	}
	if !strings.Contains(errMsg, "REDIS_URL") {
		t.Errorf("expected error to mention REDIS_URL, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "KAI_AUTH_TOKEN") {
		t.Errorf("expected error to mention KAI_AUTH_TOKEN, got: %s", errMsg)
	}
}
