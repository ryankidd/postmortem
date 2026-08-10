package timeline

import (
	"testing"
	"time"
)

func TestParseWindowAcceptsRFC3339(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	since, until, err := ParseWindow("2026-08-10T10:00:00Z", "2026-08-10T11:00:00Z", now)
	if err != nil {
		t.Fatalf("ParseWindow: %v", err)
	}
	if !since.Equal(time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("since = %v", since)
	}
	if !until.Equal(time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC)) {
		t.Errorf("until = %v", until)
	}
}

func TestParseWindowAcceptsRelativeDuration(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	since, until, err := ParseWindow("2h", "", now)
	if err != nil {
		t.Fatalf("ParseWindow: %v", err)
	}
	if !since.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("since = %v, want 2h before now", since)
	}
	if !until.Equal(now) {
		t.Errorf("until = %v, want now (default)", until)
	}
}

func TestParseWindowRejectsUntilBeforeSince(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	_, _, err := ParseWindow("1h", "2h", now)
	if err == nil {
		t.Fatal("expected error when until is before since, got nil")
	}
}

func TestParseWindowRejectsGarbage(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	if _, _, err := ParseWindow("not-a-time", "", now); err == nil {
		t.Fatal("expected error for unparseable since value")
	}
}
