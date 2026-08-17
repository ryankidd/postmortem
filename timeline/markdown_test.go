package timeline

import (
	"strings"
	"testing"
	"time"
)

func TestRenderMarkdownHeadingAndWindow(t *testing.T) {
	since := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)

	out := RenderMarkdown(nil, since, until)

	if !strings.HasPrefix(out, "# Incident timeline\n") {
		t.Fatalf("missing heading:\n%s", out)
	}
	if !strings.Contains(out, "**Window:** 2026-08-10T12:00:00Z → 2026-08-10T13:00:00Z") {
		t.Fatalf("window not rendered:\n%s", out)
	}
	if !strings.Contains(out, "_No events in this window._") {
		t.Fatalf("empty timeline note missing:\n%s", out)
	}
}

func TestRenderMarkdownEventTable(t *testing.T) {
	since := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)

	events := []Event{
		{
			Time:    time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
			Source:  "git",
			Kind:    Change,
			Summary: "9f3c1ab Add request timeout",
		},
		{
			Time:    time.Date(2026, 8, 10, 12, 16, 0, 0, time.UTC),
			Source:  "prometheus",
			Kind:    Anomaly,
			Summary: "node_load1 crossed above 5",
			Suspect: "16m after [git] 9f3c1ab Add request timeout",
		},
	}

	out := RenderMarkdown(events, since, until)

	if !strings.Contains(out, "| Time | Source | Event |\n| --- | --- | --- |\n") {
		t.Fatalf("table header missing:\n%s", out)
	}
	if !strings.Contains(out, "| 2026-08-10T12:00:00Z | git | 9f3c1ab Add request timeout |\n") {
		t.Fatalf("change row missing:\n%s", out)
	}
	wantAnomaly := "| 2026-08-10T12:16:00Z | prometheus | node_load1 crossed above 5" +
		"<br>**suspect:** 16m after [git] 9f3c1ab Add request timeout |\n"
	if !strings.Contains(out, wantAnomaly) {
		t.Fatalf("anomaly row with suspect missing:\n%s", out)
	}
}

func TestRenderMarkdownEscapesPipes(t *testing.T) {
	since := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)

	events := []Event{
		{
			Time:    since,
			Source:  "journald",
			Summary: "api.service: upstream a | b failed",
		},
	}

	out := RenderMarkdown(events, since, until)

	if !strings.Contains(out, "upstream a \\| b failed") {
		t.Fatalf("pipe not escaped in cell:\n%s", out)
	}
}
