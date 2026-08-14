// Package git provides a timeline.Source backed by a git repository,
// read via the git command line.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/ryankidd/postmortem/timeline"
)

// Field and record separators used in the --pretty/--format templates.
// Subjects and author names can contain anything a shell or a newline can,
// so records are delimited with the ASCII control characters reserved for
// exactly this instead of guessing at a safe printable delimiter.
const (
	fieldSep  = "\x1f"
	recordSep = "\x1e"
)

// logFormat lays out one commit as: short SHA, committer date as UNIX
// seconds, author name, subject.
const logFormat = "%h%x1f%ct%x1f%an%x1f%s%x1e"

// tagFormat lays out one tag as: creation date as UNIX seconds, tag name,
// the tag object's own SHA, the SHA of the commit it dereferences to
// (empty for lightweight tags), and the subject line.
const tagFormat = "%(creatordate:unix)%1f%(refname:short)%1f%(objectname:short)%1f%(*objectname:short)%1f%(contents:subject)"

// Runner invokes git with the given arguments and returns its stdout.
// Tests substitute a fixture-backed Runner in place of RunGit.
type Runner func(args []string) ([]byte, error)

// Source reads commits and tags from a git repository.
type Source struct {
	// Repo is the path to the repository to read. Empty means the
	// current working directory.
	Repo string

	// Run invokes git. Defaults to RunGit.
	Run Runner
}

// RunGit shells out to the real git binary.
func RunGit(args []string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, nil
}

func (s Source) Name() string { return "git" }

// Events returns the commits made within [since, until] plus any tags
// created in the same window.
func (s Source) Events(since, until time.Time) ([]timeline.Event, error) {
	run := s.Run
	if run == nil {
		run = RunGit
	}

	commits, err := s.commits(run, since, until)
	if err != nil {
		return nil, err
	}

	tags, err := s.tags(run, since, until)
	if err != nil {
		return nil, err
	}

	return append(commits, tags...), nil
}

func (s Source) commits(run Runner, since, until time.Time) ([]timeline.Event, error) {
	// --reverse makes git emit oldest commits first. Without it the
	// merge step's stable sort leaves commits sharing a timestamp in
	// git's default newest-first order, which reads backwards on a
	// timeline.
	out, err := run(s.args(
		"log",
		"--no-show-signature",
		"--reverse",
		"--since="+gitTime(since),
		"--until="+gitTime(until),
		"--pretty=format:"+logFormat,
	))
	if err != nil {
		return nil, s.wrap(err)
	}
	return parseCommits(out)
}

func (s Source) tags(run Runner, since, until time.Time) ([]timeline.Event, error) {
	// git tag has no date filter of its own, so every tag is listed and
	// the window is applied here.
	out, err := run(s.args("tag", "--list", "--format="+tagFormat))
	if err != nil {
		return nil, s.wrap(err)
	}
	return parseTags(out, since, until)
}

// args prefixes a git invocation with -C so the repository is selected
// without changing the process working directory.
func (s Source) args(rest ...string) []string {
	if s.Repo == "" {
		return rest
	}
	return append([]string{"-C", s.Repo}, rest...)
}

// wrap annotates an error with the repository it came from, which matters
// once more than one repository is being read.
func (s Source) wrap(err error) error {
	if s.Repo == "" {
		return err
	}
	return fmt.Errorf("%s: %w", s.Repo, err)
}

// gitTime formats t as an absolute UNIX timestamp, the one date format
// git's approxidate parser reads without reference to the local timezone.
func gitTime(t time.Time) string {
	return "@" + strconv.FormatInt(t.Unix(), 10)
}

// parseCommits parses the record-delimited output of git log.
func parseCommits(out []byte) ([]timeline.Event, error) {
	var events []timeline.Event

	for _, record := range splitRecords(out, recordSep) {
		fields := bytes.SplitN(record, []byte(fieldSep), 4)
		if len(fields) != 4 {
			return nil, fmt.Errorf("parse git commit %q: got %d fields, want 4", record, len(fields))
		}

		t, err := parseUnix(string(fields[1]))
		if err != nil {
			return nil, fmt.Errorf("parse git commit timestamp %q: %w", fields[1], err)
		}

		events = append(events, timeline.Event{
			Time:    t,
			Source:  "git",
			Kind:    timeline.Change,
			Summary: fmt.Sprintf("%s %s (%s)", fields[0], fields[3], fields[2]),
		})
	}

	return events, nil
}

// parseTags parses the line-delimited output of git tag --list, keeping
// only the tags whose creation date falls inside the window.
func parseTags(out []byte, since, until time.Time) ([]timeline.Event, error) {
	var events []timeline.Event

	for _, record := range splitRecords(out, "\n") {
		fields := bytes.SplitN(record, []byte(fieldSep), 5)
		if len(fields) != 5 {
			return nil, fmt.Errorf("parse git tag %q: got %d fields, want 5", record, len(fields))
		}

		t, err := parseUnix(string(fields[0]))
		if err != nil {
			return nil, fmt.Errorf("parse git tag timestamp %q: %w", fields[0], err)
		}
		if t.Before(since) || t.After(until) {
			continue
		}

		// Annotated tags carry their own object, so the commit SHA is
		// the dereferenced one; lightweight tags point at the commit
		// directly and leave that field empty.
		target := fields[3]
		if len(target) == 0 {
			target = fields[2]
		}

		summary := fmt.Sprintf("tag %s -> %s", fields[1], target)
		if subject := bytes.TrimSpace(fields[4]); len(subject) > 0 {
			summary += " " + string(subject)
		}

		events = append(events, timeline.Event{
			Time:    t,
			Source:  "git",
			Kind:    timeline.Change,
			Summary: summary,
		})
	}

	return events, nil
}

// splitRecords splits output on sep and drops the blank records left by
// trailing separators and empty output.
func splitRecords(out []byte, sep string) [][]byte {
	var records [][]byte
	for _, r := range bytes.Split(out, []byte(sep)) {
		r = bytes.TrimSpace(r)
		if len(r) == 0 {
			continue
		}
		records = append(records, r)
	}
	return records
}

func parseUnix(s string) (time.Time, error) {
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0).UTC(), nil
}
