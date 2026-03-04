package cache

import (
	"testing"
	"time"
)

func TestTTLToMidnight(t *testing.T) {
	ttl := TTLToMidnight()

	if ttl <= 0 {
		t.Error("expected positive TTL")
	}
	if ttl > 24*time.Hour {
		t.Errorf("expected TTL <= 24h, got %v", ttl)
	}
}
