package clock

import (
	"testing"
	"time"
)

func TestRealClock(t *testing.T) {
	c := NewRealClock()
	now := c.Now()
	if now.IsZero() {
		t.Errorf("expected non-zero time")
	}

	time.Sleep(2 * time.Millisecond)
	since := c.Since(now)
	if since <= 0 {
		t.Errorf("expected positive duration, got %v", since)
	}

	c.Sleep(1 * time.Millisecond)
}

func TestMockClock(t *testing.T) {
	fixed := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	mc := NewMockClock(fixed)

	if !mc.Now().Equal(fixed) {
		t.Errorf("expected %v, got %v", fixed, mc.Now())
	}

	past := fixed.Add(-5 * time.Minute)
	if mc.Since(past) != 5*time.Minute {
		t.Errorf("expected 5m, got %v", mc.Since(past))
	}

	mc.Sleep(10 * time.Second)
	expectedAfter := fixed.Add(10 * time.Second)
	if !mc.Now().Equal(expectedAfter) {
		t.Errorf("expected %v after sleep, got %v", expectedAfter, mc.Now())
	}
}
