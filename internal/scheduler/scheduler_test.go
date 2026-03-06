package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/comu/api/internal/config"
)

func TestNextRunTime_IsFuture(t *testing.T) {
	next := nextRunTime()
	if !next.After(time.Now()) {
		t.Errorf("expected next run time to be in the future, got %v", next)
	}
}

func TestNextRunTime_IsAt0010WIB(t *testing.T) {
	next := nextRunTime()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	nextWIB := next.In(loc)

	if nextWIB.Hour() != 0 || nextWIB.Minute() != 10 {
		t.Errorf("expected 00:10 WIB, got %02d:%02d", nextWIB.Hour(), nextWIB.Minute())
	}
}

func TestRefreshToken_ChecksExpiryAfterRefreshUsingEffectiveToken(t *testing.T) {
	originalTryRefresh := tryRefreshFunc
	originalCheckExpiry := checkExpiryFunc
	defer func() {
		tryRefreshFunc = originalTryRefresh
		checkExpiryFunc = originalCheckExpiry
	}()

	var checked atomic.Bool
	cfg := &config.Config{KAIAuthToken: "stale-token"}

	tryRefreshFunc = func(ctx context.Context, current string, rotate func(string), botToken string, adminID int64) (bool, error) {
		if current != "stale-token" {
			t.Fatalf("TryRefresh current token = %q, want stale-token", current)
		}
		rotate("fresh-token")
		return true, nil
	}
	checkExpiryFunc = func(ctx context.Context, token, botToken string, adminID int64, warnWithin time.Duration) bool {
		checked.Store(true)
		if token != "fresh-token" {
			t.Fatalf("CheckExpiry token = %q, want fresh-token", token)
		}
		return false
	}

	refreshToken(context.Background(), cfg)

	if !checked.Load() {
		t.Fatal("expected CheckExpiry to be called")
	}
}
