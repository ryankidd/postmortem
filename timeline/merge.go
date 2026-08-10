package timeline

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Build queries every source for the window and returns a single
// chronologically sorted timeline.
func Build(sources []Source, since, until time.Time) ([]Event, error) {
	var events []Event
	for _, src := range sources {
		got, err := src.Events(since, until)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", src.Name(), err)
		}
		events = append(events, got...)
	}

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})

	return events, nil
}

// Render formats a timeline as plain text, one line per event.
func Render(events []Event) string {
	var b strings.Builder
	for _, e := range events {
		fmt.Fprintf(&b, "%s [%s] %s\n", e.Time.Format(time.RFC3339), e.Source, e.Summary)
	}
	return b.String()
}
