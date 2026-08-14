package timeline

import (
	"testing"
	"time"
)

func TestCorrelateNamesNearestPrecedingChange(t *testing.T) {
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	events := []Event{
		{Time: base, Source: "git", Kind: Change, Summary: "9f3c1ab Add request timeout"},
		{Time: base.Add(3 * time.Minute), Source: "git", Kind: Change, Summary: "5b2d4e7 Raise the pool ceiling"},
		{Time: base.Add(15 * time.Minute), Source: "prometheus", Kind: Anomaly, Summary: "node_load1 crossed above 5"},
	}

	got := Correlate(events)

	want := "12m after [git] 5b2d4e7 Raise the pool ceiling"
	if got[2].Suspect != want {
		t.Fatalf("Suspect = %q, want %q", got[2].Suspect, want)
	}
	// The changes themselves are never given a suspect.
	if got[0].Suspect != "" || got[1].Suspect != "" {
		t.Fatalf("change events annotated: %q, %q", got[0].Suspect, got[1].Suspect)
	}
}

func TestCorrelateLeavesAnomalyWithNoPrecedingChangeAlone(t *testing.T) {
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	events := []Event{
		{Time: base, Source: "prometheus", Kind: Anomaly, Summary: "load crossed above 5"},
		{Time: base.Add(5 * time.Minute), Source: "git", Kind: Change, Summary: "abc later commit"},
	}

	got := Correlate(events)

	if got[0].Suspect != "" {
		t.Fatalf("anomaly with no preceding change got Suspect %q", got[0].Suspect)
	}
}

func TestCorrelateIgnoresChangeAtSameInstant(t *testing.T) {
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	events := []Event{
		{Time: base.Add(-2 * time.Minute), Source: "git", Kind: Change, Summary: "earlier commit"},
		{Time: base, Source: "git", Kind: Change, Summary: "simultaneous commit"},
		{Time: base, Source: "prometheus", Kind: Anomaly, Summary: "load crossed above 5"},
	}

	got := Correlate(events)

	// A change sharing the anomaly's timestamp does not "precede" it, so the
	// strictly earlier change is the suspect.
	want := "2m after [git] earlier commit"
	if got[2].Suspect != want {
		t.Fatalf("Suspect = %q, want %q", got[2].Suspect, want)
	}
}

func TestCorrelateIgnoresNeutralEvents(t *testing.T) {
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	events := []Event{
		{Time: base, Source: "git", Kind: Change, Summary: "the deploy"},
		{Time: base.Add(4 * time.Minute), Source: "journald", Kind: Neutral, Summary: "routine log line"},
		{Time: base.Add(10 * time.Minute), Source: "prometheus", Kind: Anomaly, Summary: "load crossed above 5"},
	}

	got := Correlate(events)

	// The neutral line between change and anomaly is skipped, not treated as
	// the suspect.
	want := "10m after [git] the deploy"
	if got[2].Suspect != want {
		t.Fatalf("Suspect = %q, want %q", got[2].Suspect, want)
	}
}

func TestCorrelateDoesNotMutateInput(t *testing.T) {
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	events := []Event{
		{Time: base, Source: "git", Kind: Change, Summary: "deploy"},
		{Time: base.Add(time.Minute), Source: "prometheus", Kind: Anomaly, Summary: "crossed"},
	}

	Correlate(events)

	if events[1].Suspect != "" {
		t.Fatalf("Correlate mutated its input: %q", events[1].Suspect)
	}
}

func TestFormatDelta(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{12 * time.Minute, "12m"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{time.Hour, "1h"},
		{time.Hour + 5*time.Minute + 3*time.Second, "1h5m3s"},
		{0, "0s"},
	} {
		if got := formatDelta(tc.d); got != tc.want {
			t.Errorf("formatDelta(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
