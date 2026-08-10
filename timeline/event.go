// Package timeline builds merged, time-ordered incident timelines from
// multiple sources (journald, git, Prometheus, ...).
package timeline

import "time"

// Event is one point on the timeline, contributed by some Source.
type Event struct {
	Time    time.Time
	Source  string
	Summary string
}

// Source produces the events it knows about within [since, until].
type Source interface {
	Name() string
	Events(since, until time.Time) ([]Event, error)
}
