package timeline

import (
	"testing"
	"time"
)

type fakeSource struct {
	name   string
	events []Event
}

func (f fakeSource) Name() string { return f.name }

func (f fakeSource) Events(since, until time.Time) ([]Event, error) {
	var out []Event
	for _, e := range f.events {
		if !e.Time.Before(since) && e.Time.Before(until) {
			out = append(out, e)
		}
	}
	return out, nil
}

func TestBuildMergesAndSortsAcrossSources(t *testing.T) {
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	git := fakeSource{
		name: "git",
		events: []Event{
			{Time: base.Add(5 * time.Minute), Source: "git", Summary: "deploy v1.2.3"},
		},
	}
	journal := fakeSource{
		name: "journald",
		events: []Event{
			{Time: base.Add(10 * time.Minute), Source: "journald", Summary: "unit api.service failed"},
			{Time: base.Add(-time.Hour), Source: "journald", Summary: "outside window, must be excluded"},
		},
	}

	since, until := base, base.Add(time.Hour)
	got, err := Build([]Source{git, journal}, since, until)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	if got[0].Source != "git" || got[1].Source != "journald" {
		t.Fatalf("events not sorted chronologically: %+v", got)
	}
}

func TestRenderFormatsEachEventOnOneLine(t *testing.T) {
	events := []Event{
		{Time: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), Source: "git", Summary: "deploy v1.2.3"},
	}

	out := Render(events)
	want := "2026-08-10T12:00:00Z [git] deploy v1.2.3\n"
	if out != want {
		t.Fatalf("Render() = %q, want %q", out, want)
	}
}

func TestRenderIncludesSuspectAnnotation(t *testing.T) {
	events := []Event{
		{
			Time:    time.Date(2026, 8, 10, 12, 16, 0, 0, time.UTC),
			Source:  "prometheus",
			Kind:    Anomaly,
			Summary: "node_load1 crossed above 5",
			Suspect: "16m after [git] 9f3c1ab Add request timeout",
		},
	}

	out := Render(events)
	want := "2026-08-10T12:16:00Z [prometheus] node_load1 crossed above 5\n" +
		"    suspect: 16m after [git] 9f3c1ab Add request timeout\n"
	if out != want {
		t.Fatalf("Render() = %q, want %q", out, want)
	}
}
