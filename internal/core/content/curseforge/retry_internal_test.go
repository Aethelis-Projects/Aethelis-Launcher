package curseforge

import (
	"net/http"
	"testing"
	"time"
)

// v0.7.2 coverage margin: the 429-backoff parser (used by every CF request)
// previously had no direct unit tests.
func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if d, ok := parseRetryAfter("42", now); !ok || d != 42*time.Second {
		t.Errorf("seconds: got %v,%v", d, ok)
	}
	if d, ok := parseRetryAfter(" 7 ", now); !ok || d != 7*time.Second {
		t.Errorf("trimmed seconds: got %v,%v", d, ok)
	}
	if _, ok := parseRetryAfter("", now); ok {
		t.Error("empty must not parse")
	}
	if _, ok := parseRetryAfter("-5", now); ok {
		t.Error("negative seconds must not parse")
	}
	if _, ok := parseRetryAfter("not-a-date", now); ok {
		t.Error("garbage must not parse")
	}
	if d, ok := parseRetryAfter(now.Add(90*time.Second).Format(http.TimeFormat), now); !ok || d < 60*time.Second || d > 90*time.Second {
		t.Errorf("http-date: got %v,%v", d, ok)
	}
	if d, ok := parseRetryAfter(now.Add(-time.Hour).Format(http.TimeFormat), now); !ok || d != 0 {
		t.Errorf("past date must clamp to 0: got %v,%v", d, ok)
	}
	if d, ok := parseRetryAfter(now.Add(30*time.Second).Format(time.RFC850), now); !ok || d <= 0 {
		t.Errorf("rfc850: got %v,%v", d, ok)
	}
}

func TestBuiltinAPIKeyAccessors(t *testing.T) {
	old := GetBuiltinAPIKey()
	t.Cleanup(func() { SetBuiltinAPIKey(old) })
	SetBuiltinAPIKey("")
	if HasBuiltinKey() || GetBuiltinAPIKey() != "" {
		t.Fatal("empty key must report HasBuiltinKey=false")
	}
	SetBuiltinAPIKey("secret-key")
	if !HasBuiltinKey() || GetBuiltinAPIKey() != "secret-key" {
		t.Fatal("configured key must round-trip")
	}
}
