package timeline

import (
	"fmt"
	"time"
)

// ParseWindow resolves --since/--until flag values into a concrete time
// range. Each value is either an RFC3339 timestamp or a duration (e.g.
// "2h", "30m") measured back from now. An empty until defaults to now.
func ParseWindow(since, until string, now time.Time) (time.Time, time.Time, error) {
	sinceTime, err := parseWindowArg(since, now)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("since: %w", err)
	}

	untilTime := now
	if until != "" {
		untilTime, err = parseWindowArg(until, now)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("until: %w", err)
		}
	}

	if !untilTime.After(sinceTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("until (%s) must be after since (%s)", untilTime, sinceTime)
	}

	return sinceTime, untilTime, nil
}

func parseWindowArg(v string, now time.Time) (time.Time, error) {
	if v == "" {
		return time.Time{}, fmt.Errorf("value required")
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if d, err := time.ParseDuration(v); err == nil {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("%q is not an RFC3339 timestamp or a duration", v)
}
