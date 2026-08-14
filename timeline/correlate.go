package timeline

import (
	"fmt"
	"strings"
	"time"
)

// Correlate annotates every anomaly with the nearest change that precedes it
// in time, recording the result in the anomaly's Suspect field. It expects
// events in chronological order, as returned by Build, and leaves the
// ordering untouched.
//
// This is a heuristic, not a proof: naming the nearest preceding change is a
// correlation, not evidence of causation. A change that happens to land just
// before an anomaly is flagged whether or not it had anything to do with it,
// and a real cause that predates a closer, unrelated change is missed.
func Correlate(events []Event) []Event {
	out := make([]Event, len(events))
	copy(out, events)

	for i := range out {
		if out[i].Kind != Anomaly {
			continue
		}
		if c := nearestPrecedingChange(out, i); c != nil {
			delta := out[i].Time.Sub(c.Time)
			out[i].Suspect = fmt.Sprintf("%s after [%s] %s",
				formatDelta(delta), c.Source, c.Summary)
		}
	}

	return out
}

// nearestPrecedingChange walks backwards from the anomaly at index i and
// returns the closest earlier change event, or nil if none precedes it. A
// change sharing the anomaly's exact timestamp does not count as preceding
// it, so a strictly earlier change is preferred over a simultaneous one.
func nearestPrecedingChange(events []Event, i int) *Event {
	for j := i - 1; j >= 0; j-- {
		if events[j].Kind == Change && events[j].Time.Before(events[i].Time) {
			return &events[j]
		}
	}
	return nil
}

// formatDelta renders a gap between two events compactly: hours, minutes and
// seconds, dropping any zero component but always keeping at least one.
func formatDelta(d time.Duration) string {
	d = d.Round(time.Second)

	h := int(d / time.Hour)
	m := int(d % time.Hour / time.Minute)
	s := int(d % time.Minute / time.Second)

	var b strings.Builder
	if h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m > 0 {
		fmt.Fprintf(&b, "%dm", m)
	}
	if s > 0 || b.Len() == 0 {
		fmt.Fprintf(&b, "%ds", s)
	}
	return b.String()
}
