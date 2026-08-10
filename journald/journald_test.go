package journald

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestParseEntriesFromFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	got, err := parseEntries(data)
	if err != nil {
		t.Fatalf("parseEntries: %v", err)
	}

	want := []struct {
		time    time.Time
		summary string
	}{
		{time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), "api.service: Started API service."},
		{time.Date(2026, 8, 10, 12, 0, 30, 0, time.UTC), "api.service: connection from 203.0.113.10 accepted"},
		{time.Date(2026, 8, 10, 12, 1, 5, 0, time.UTC), "kernel: disk I/O error on sdb"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}

	for i, w := range want {
		if !got[i].Time.Equal(w.time) {
			t.Errorf("event %d: Time = %v, want %v", i, got[i].Time, w.time)
		}
		if got[i].Source != "journald" {
			t.Errorf("event %d: Source = %q, want %q", i, got[i].Source, "journald")
		}
		if got[i].Summary != w.summary {
			t.Errorf("event %d: Summary = %q, want %q", i, got[i].Summary, w.summary)
		}
	}
}

func TestParseEntriesRejectsMalformedLine(t *testing.T) {
	_, err := parseEntries([]byte(`{"__REALTIME_TIMESTAMP": not json}`))
	if err == nil {
		t.Fatal("parseEntries: want error for malformed line, got nil")
	}
}

func TestParseEntriesSkipsBlankLines(t *testing.T) {
	got, err := parseEntries([]byte("\n\n  \n"))
	if err != nil {
		t.Fatalf("parseEntries: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d events, want 0: %+v", len(got), got)
	}
}

func TestJournalTimeFormatsAsAtEpoch(t *testing.T) {
	tm := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	got := journalTime(tm)
	want := "@1786363200"
	if got != want {
		t.Fatalf("journalTime() = %q, want %q", got, want)
	}
}

func TestSourceEventsInvokesJournalctlWithSinceUntilAndUnit(t *testing.T) {
	fixture, err := os.ReadFile("testdata/sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var gotArgs []string
	src := Source{
		Unit: "api.service",
		Run: func(args []string) ([]byte, error) {
			gotArgs = args
			return fixture, nil
		},
	}

	since := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)

	events, err := src.Events(since, until)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}

	wantArgs := []string{
		"-o", "json",
		"--no-pager",
		"--since", "@1786363200",
		"--until", "@1786366800",
		"-u", "api.service",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("journalctl args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestSourceEventsOmitsUnitFlagWhenUnset(t *testing.T) {
	var gotArgs []string
	src := Source{
		Run: func(args []string) ([]byte, error) {
			gotArgs = args
			return []byte(""), nil
		},
	}

	since := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)

	if _, err := src.Events(since, until); err != nil {
		t.Fatalf("Events: %v", err)
	}

	for _, a := range gotArgs {
		if a == "-u" {
			t.Fatalf("journalctl args = %v, want no -u flag", gotArgs)
		}
	}
}

func TestSourceName(t *testing.T) {
	if got := (Source{}).Name(); got != "journald" {
		t.Fatalf("Name() = %q, want %q", got, "journald")
	}
}
