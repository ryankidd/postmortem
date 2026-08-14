// Package journald provides a timeline.Source backed by the systemd
// journal, read via journalctl.
package journald

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ryankidd/postmortem/timeline"
)

// Runner invokes journalctl with the given arguments and returns its
// stdout. Tests substitute a fixture-backed Runner in place of
// RunJournalctl.
type Runner func(args []string) ([]byte, error)

// Source reads events from the systemd journal via journalctl.
type Source struct {
	// Unit, if set, restricts output to a single systemd unit
	// (journalctl -u). Empty means no unit filter.
	Unit string

	// Run invokes journalctl. Defaults to RunJournalctl.
	Run Runner
}

// RunJournalctl shells out to the real journalctl binary.
func RunJournalctl(args []string) ([]byte, error) {
	cmd := exec.Command("journalctl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, nil
}

func (s Source) Name() string { return "journald" }

// Events runs journalctl scoped to [since, until] and parses its JSON
// output into timeline events.
func (s Source) Events(since, until time.Time) ([]timeline.Event, error) {
	run := s.Run
	if run == nil {
		run = RunJournalctl
	}

	args := []string{
		"-o", "json",
		"--no-pager",
		"--since", journalTime(since),
		"--until", journalTime(until),
	}
	if s.Unit != "" {
		args = append(args, "-u", s.Unit)
	}

	out, err := run(args)
	if err != nil {
		return nil, err
	}

	return parseEntries(out)
}

// journalTime formats t the way journalctl's --since/--until expect: an
// "@<seconds>" absolute UNIX timestamp. This sidesteps the ambiguity of
// journalctl's own local-time date parser entirely.
func journalTime(t time.Time) string {
	return "@" + strconv.FormatInt(t.Unix(), 10)
}

// entry mirrors the subset of `journalctl -o json` fields this source
// cares about. journald prefixes its own well-known fields with
// underscores; MESSAGE and PRIORITY come from the client/syslog side.
type entry struct {
	RealtimeTimestamp string          `json:"__REALTIME_TIMESTAMP"`
	Message           json.RawMessage `json:"MESSAGE"`
	Unit              string          `json:"_SYSTEMD_UNIT"`
	SyslogIdentifier  string          `json:"SYSLOG_IDENTIFIER"`
	Priority          string          `json:"PRIORITY"`
}

// parseEntries parses journalctl's `-o json` output: one compact JSON
// object per line, one line per journal entry.
func parseEntries(out []byte) ([]timeline.Event, error) {
	var events []timeline.Event

	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var e entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("parse journal entry: %w", err)
		}

		t, err := parseTimestamp(e.RealtimeTimestamp)
		if err != nil {
			return nil, fmt.Errorf("parse journal timestamp %q: %w", e.RealtimeTimestamp, err)
		}

		events = append(events, timeline.Event{
			Time:    t,
			Source:  "journald",
			Kind:    classify(e),
			Summary: summarize(e),
		})
	}

	return events, nil
}

// parseTimestamp converts __REALTIME_TIMESTAMP, which journald reports as
// a decimal string of microseconds since the UNIX epoch.
func parseTimestamp(s string) (time.Time, error) {
	usec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMicro(usec).UTC(), nil
}

// errorPriority is the highest syslog priority value (least severe) still
// treated as error-ish. syslog numbers severity in reverse: 3 is err, and
// 2, 1, 0 are crit, alert and emerg.
const errorPriority = 3

// lifecycleVerbs are the leading words systemd uses when it logs a unit
// changing state. A journal line from systemd itself that opens with one of
// these is treated as a change to the system.
var lifecycleVerbs = []string{
	"Started",
	"Starting",
	"Stopped",
	"Stopping",
	"Restarted",
	"Reloaded",
	"Reloading",
}

// classify sorts a journal entry into a timeline.Kind. An error-level (or
// worse) entry is an anomaly; a systemd unit start/stop line is a change;
// anything else is neutral. Anomaly wins when both would apply.
func classify(e entry) timeline.Kind {
	if isErrorPriority(e.Priority) {
		return timeline.Anomaly
	}
	if isLifecycle(e) {
		return timeline.Change
	}
	return timeline.Neutral
}

// isErrorPriority reports whether a PRIORITY field names err or worse. A
// missing or unparseable priority counts as not error-ish.
func isErrorPriority(priority string) bool {
	p, err := strconv.Atoi(priority)
	if err != nil {
		return false
	}
	return p <= errorPriority
}

// isLifecycle reports whether an entry is systemd announcing a unit change
// of state, matched on the leading verb of the message.
func isLifecycle(e entry) bool {
	if e.SyslogIdentifier != "systemd" {
		return false
	}
	msg := decodeMessage(e.Message)
	for _, verb := range lifecycleVerbs {
		if strings.HasPrefix(msg, verb+" ") {
			return true
		}
	}
	return false
}

// summarize builds a one-line summary, prefixed with the unit (falling
// back to the syslog identifier) when journald reports one.
func summarize(e entry) string {
	msg := decodeMessage(e.Message)

	label := e.Unit
	if label == "" {
		label = e.SyslogIdentifier
	}
	if label == "" {
		return msg
	}
	return fmt.Sprintf("%s: %s", label, msg)
}

// decodeMessage handles both shapes journald uses for MESSAGE: a plain
// UTF-8 string in the common case, or an array of byte values when the
// original message wasn't valid UTF-8.
func decodeMessage(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var nums []byte
	if err := json.Unmarshal(raw, &nums); err == nil {
		return string(nums)
	}

	return string(raw)
}
