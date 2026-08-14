// Package timeline builds merged, time-ordered incident timelines from
// multiple sources (journald, git, Prometheus, ...).
package timeline

import "time"

// Kind categorises an event by the role it plays in an incident. Sources
// classify their own events, since only they know what a given line means.
type Kind int

const (
	// Neutral is the zero value: an event that is neither a change nor an
	// anomaly, and so takes no part in correlation.
	Neutral Kind = iota
	// Change marks something that altered the system — a deploy, a commit,
	// a tag, a unit starting or stopping.
	Change
	// Anomaly marks something that broke — a metric crossing a threshold,
	// an error-level log line.
	Anomaly
)

// Event is one point on the timeline, contributed by some Source.
type Event struct {
	Time    time.Time
	Source  string
	Kind    Kind
	Summary string

	// Suspect is filled in by Correlate on anomaly events. It names the
	// nearest change that preceded the anomaly, together with the delay
	// between them.
	Suspect string
}

// Source produces the events it knows about within [since, until].
type Source interface {
	Name() string
	Events(since, until time.Time) ([]Event, error)
}
