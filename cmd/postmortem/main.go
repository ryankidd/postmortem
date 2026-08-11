// Command postmortem builds a merged incident timeline from journald,
// git, and Prometheus over a given time window.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ryankidd/postmortem/git"
	"github.com/ryankidd/postmortem/journald"
	"github.com/ryankidd/postmortem/timeline"
)

// repoPaths collects a repeatable --git-repo flag.
type repoPaths []string

func (p *repoPaths) String() string { return strings.Join(*p, ",") }

func (p *repoPaths) Set(v string) error {
	if v == "" {
		return fmt.Errorf("repository path required")
	}
	*p = append(*p, v)
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "postmortem:", err)
		os.Exit(1)
	}
}

func run(args []string, out *os.File) error {
	fs := flag.NewFlagSet("postmortem", flag.ContinueOnError)
	since := fs.String("since", "1h", "start of the window (RFC3339 timestamp or duration back from now)")
	until := fs.String("until", "", "end of the window (RFC3339 timestamp, default now)")
	unit := fs.String("unit", "", "restrict journald events to this systemd unit (default: all units)")
	var repos repoPaths
	fs.Var(&repos, "git-repo", "include commits and tags from this git repository (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	sinceTime, untilTime, err := timeline.ParseWindow(*since, *until, time.Now())
	if err != nil {
		return err
	}

	sources := []timeline.Source{
		journald.Source{Unit: *unit},
	}
	for _, repo := range repos {
		sources = append(sources, git.Source{Repo: repo})
	}

	events, err := timeline.Build(sources, sinceTime, untilTime)
	if err != nil {
		return err
	}

	fmt.Fprint(out, timeline.Render(events))
	return nil
}
