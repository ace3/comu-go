package scheduler

import (
	"testing"
	"time"
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
