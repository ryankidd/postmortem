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
	"github.com/ryankidd/postmortem/prometheus"
	"github.com/ryankidd/postmortem/timeline"
)

// stringList collects a repeatable string flag.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	if v == "" {
		return fmt.Errorf("value required")
	}
	*l = append(*l, v)
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
	var repos stringList
	fs.Var(&repos, "git-repo", "include commits and tags from this git repository (repeatable)")
	promURL := fs.String("prom-url", "", "base URL of a Prometheus server to query (e.g. http://localhost:9090)")
	var promQueries stringList
	fs.Var(&promQueries, "prom-query", "PromQL expression to range-query for threshold crossings (repeatable)")
	promThreshold := fs.Float64("prom-threshold", 0, "value a Prometheus series must cross to raise an event")
	promStep := fs.Duration("prom-step", time.Minute, "resolution of the Prometheus range query")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(promQueries) > 0 && *promURL == "" {
		return fmt.Errorf("--prom-query requires --prom-url")
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
	for _, query := range promQueries {
		sources = append(sources, prometheus.Source{
			URL:       *promURL,
			Query:     query,
			Threshold: *promThreshold,
			Step:      *promStep,
		})
	}

	events, err := timeline.Build(sources, sinceTime, untilTime)
	if err != nil {
		return err
	}

	events = timeline.Correlate(events)

	fmt.Fprint(out, timeline.Render(events))
	return nil
}
