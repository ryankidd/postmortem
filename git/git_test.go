package git

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

var (
	windowSince = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	windowUntil = time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

// fixtureRunner answers a git invocation with the recorded output for the
// subcommand being run.
func fixtureRunner(t *testing.T, calls *[][]string) Runner {
	t.Helper()
	return func(args []string) ([]byte, error) {
		*calls = append(*calls, args)
		for _, a := range args {
			switch a {
			case "log":
				return readFixture(t, "log.txt"), nil
			case "tag":
				return readFixture(t, "tags.txt"), nil
			}
		}
		t.Fatalf("unexpected git invocation: %v", args)
		return nil, nil
	}
}

func TestParseCommitsFromFixture(t *testing.T) {
	got, err := parseCommits(readFixture(t, "log.txt"))
	if err != nil {
		t.Fatalf("parseCommits: %v", err)
	}

	want := []struct {
		time    time.Time
		summary string
	}{
		{time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), "9f3c1ab Add request timeout to the API client (Ada Lovelace)"},
		{time.Date(2026, 8, 10, 12, 15, 0, 0, time.UTC), "5b2d4e7 Raise the connection pool ceiling (Grace Hopper)"},
		{time.Date(2026, 8, 10, 12, 45, 0, 0, time.UTC), `c81a06d Revert "Raise the connection pool ceiling" (Ada Lovelace)`},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if !got[i].Time.Equal(w.time) {
			t.Errorf("event %d: Time = %v, want %v", i, got[i].Time, w.time)
		}
		if got[i].Source != "git" {
			t.Errorf("event %d: Source = %q, want %q", i, got[i].Source, "git")
		}
		if got[i].Summary != w.summary {
			t.Errorf("event %d: Summary = %q, want %q", i, got[i].Summary, w.summary)
		}
	}
}

func TestParseTagsKeepsWindowAndResolvesTargets(t *testing.T) {
	got, err := parseTags(readFixture(t, "tags.txt"), windowSince, windowUntil)
	if err != nil {
		t.Fatalf("parseTags: %v", err)
	}

	want := []string{
		// A lightweight tag has no object of its own to dereference,
		// so its own SHA is the commit SHA.
		"tag deploy-2026-08-10 -> 9f3c1ab Add request timeout to the API client",
		// An annotated tag reports the commit it dereferences to, not
		// the SHA of the tag object.
		"tag v1.4.0 -> 5b2d4e7 Release 1.4.0",
	}

	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Summary != w {
			t.Errorf("event %d: Summary = %q, want %q", i, got[i].Summary, w)
		}
		if got[i].Source != "git" {
			t.Errorf("event %d: Source = %q, want %q", i, got[i].Source, "git")
		}
	}
}

func TestParseTagsOmitsEmptySubject(t *testing.T) {
	got, err := parseTags([]byte("1786363200\x1fv2.0.0\x1fabc1234\x1f\x1f\n"), windowSince, windowUntil)
	if err != nil {
		t.Fatalf("parseTags: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
	if want := "tag v2.0.0 -> abc1234"; got[0].Summary != want {
		t.Fatalf("Summary = %q, want %q", got[0].Summary, want)
	}
}

func TestParseRejectsMalformedRecords(t *testing.T) {
	if _, err := parseCommits([]byte("abc1234\x1fnot-a-timestamp\x1fA\x1fsubject\x1e")); err == nil {
		t.Error("parseCommits: want error for unparseable timestamp, got nil")
	}
	if _, err := parseCommits([]byte("abc1234\x1f1786363200\x1e")); err == nil {
		t.Error("parseCommits: want error for short record, got nil")
	}
	if _, err := parseTags([]byte("1786363200\x1fv1.0.0\n"), windowSince, windowUntil); err == nil {
		t.Error("parseTags: want error for short record, got nil")
	}
}

func TestParseHandlesEmptyOutput(t *testing.T) {
	commits, err := parseCommits(nil)
	if err != nil {
		t.Fatalf("parseCommits: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("got %d commits, want 0", len(commits))
	}

	tags, err := parseTags([]byte("\n"), windowSince, windowUntil)
	if err != nil {
		t.Fatalf("parseTags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("got %d tags, want 0", len(tags))
	}
}

func TestSourceEventsMergesCommitsAndTags(t *testing.T) {
	var calls [][]string
	src := Source{Repo: "/srv/api", Run: fixtureRunner(t, &calls)}

	events, err := src.Events(windowSince, windowUntil)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("got %d events, want 5: %+v", len(events), events)
	}

	if len(calls) != 2 {
		t.Fatalf("got %d git invocations, want 2: %v", len(calls), calls)
	}

	wantLog := []string{
		"-C", "/srv/api",
		"log",
		"--no-show-signature",
		"--reverse",
		"--since=@1786363200",
		"--until=@1786366800",
		"--pretty=format:" + logFormat,
	}
	if !reflect.DeepEqual(calls[0], wantLog) {
		t.Errorf("git log args = %v, want %v", calls[0], wantLog)
	}

	wantTag := []string{"-C", "/srv/api", "tag", "--list", "--format=" + tagFormat}
	if !reflect.DeepEqual(calls[1], wantTag) {
		t.Errorf("git tag args = %v, want %v", calls[1], wantTag)
	}
}

func TestSourceEventsOmitsRepoFlagWhenUnset(t *testing.T) {
	var calls [][]string
	src := Source{Run: fixtureRunner(t, &calls)}

	if _, err := src.Events(windowSince, windowUntil); err != nil {
		t.Fatalf("Events: %v", err)
	}
	for _, args := range calls {
		if len(args) > 0 && args[0] == "-C" {
			t.Fatalf("git args = %v, want no -C flag", args)
		}
	}
}

func TestSourceEventsReportsRepoInErrors(t *testing.T) {
	src := Source{
		Repo: "/srv/api",
		Run: func([]string) ([]byte, error) {
			return nil, errFailed{}
		},
	}

	_, err := src.Events(windowSince, windowUntil)
	if err == nil {
		t.Fatal("Events: want error, got nil")
	}
	if !strings.Contains(err.Error(), "/srv/api") {
		t.Fatalf("error = %q, want it to name the repository", err)
	}
}

type errFailed struct{}

func (errFailed) Error() string { return "exit status 128" }

func TestGitTimeFormatsAsAtEpoch(t *testing.T) {
	if got, want := gitTime(windowSince), "@1786363200"; got != want {
		t.Fatalf("gitTime() = %q, want %q", got, want)
	}
}

func TestSourceName(t *testing.T) {
	if got := (Source{}).Name(); got != "git" {
		t.Fatalf("Name() = %q, want %q", got, "git")
	}
}
