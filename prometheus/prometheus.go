// Package prometheus provides a timeline.Source backed by the Prometheus
// HTTP API. It runs a range query over the incident window and flags the
// points where a series crosses a configured threshold.
package prometheus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryankidd/postmortem/timeline"
)

// defaultStep is the query resolution used when a Source leaves Step unset.
const defaultStep = time.Minute

// Doer performs an HTTP request. *http.Client satisfies it; tests substitute
// a fixture-backed Doer in place of the real Prometheus server.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Source runs a Prometheus range query and reports the timestamps at which
// the resulting series cross a threshold.
type Source struct {
	// URL is the base URL of the Prometheus server, e.g.
	// "http://prometheus:9090".
	URL string

	// Query is the PromQL expression to evaluate over the window.
	Query string

	// Threshold is the value a series must cross to produce an event.
	Threshold float64

	// Step is the range query resolution. Zero means defaultStep.
	Step time.Duration

	// Client performs the HTTP request. Defaults to http.DefaultClient.
	Client Doer
}

func (s Source) Name() string { return "prometheus" }

// Events runs the range query over [since, until] and returns an event for
// every threshold crossing in the returned series.
func (s Source) Events(since, until time.Time) ([]timeline.Event, error) {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}

	body, err := s.queryRange(client, since, until)
	if err != nil {
		return nil, err
	}

	return detect(body, s.Threshold, s.Query)
}

// queryRange issues the query_range request and returns its response body.
func (s Source) queryRange(client Doer, since, until time.Time) ([]byte, error) {
	step := s.Step
	if step <= 0 {
		step = defaultStep
	}

	endpoint, err := url.Parse(strings.TrimRight(s.URL, "/") + "/api/v1/query_range")
	if err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}
	q := url.Values{}
	q.Set("query", s.Query)
	q.Set("start", formatTime(since))
	q.Set("end", formatTime(until))
	q.Set("step", formatStep(step))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prometheus: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("prometheus: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus: %s: %s", resp.Status, bytes.TrimSpace(body))
	}

	return body, nil
}

// formatTime renders an instant the way the query_range start/end parameters
// expect it: a UNIX timestamp in seconds.
func formatTime(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

// formatStep renders a resolution as the seconds count the step parameter
// expects.
func formatStep(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
}

// response mirrors the subset of the query_range envelope this source reads.
type response struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
	Data      struct {
		ResultType string   `json:"resultType"`
		Result     []series `json:"result"`
	} `json:"data"`
}

// series is one matrix series: its label set and its samples over the range.
type series struct {
	Metric map[string]string `json:"metric"`
	Values []sample          `json:"values"`
}

// sample is one [timestamp, value] pair. Prometheus encodes it as a
// two-element array of a float second timestamp and a string-quoted value.
type sample struct {
	Time  time.Time
	Value float64
}

func (s *sample) UnmarshalJSON(b []byte) error {
	var pair []json.RawMessage
	if err := json.Unmarshal(b, &pair); err != nil {
		return err
	}
	if len(pair) != 2 {
		return fmt.Errorf("sample: got %d elements, want 2", len(pair))
	}

	var ts float64
	if err := json.Unmarshal(pair[0], &ts); err != nil {
		return fmt.Errorf("sample timestamp: %w", err)
	}

	var raw string
	if err := json.Unmarshal(pair[1], &raw); err != nil {
		return fmt.Errorf("sample value: %w", err)
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("sample value %q: %w", raw, err)
	}

	sec := int64(ts)
	s.Time = time.Unix(sec, int64((ts-float64(sec))*1e9)).UTC()
	s.Value = v
	return nil
}

// detect parses a query_range response and returns an event for every point
// where a series crosses the threshold, in either direction.
func detect(body []byte, threshold float64, query string) ([]timeline.Event, error) {
	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parse prometheus response: %w", err)
	}
	if r.Status != "success" {
		msg := r.Error
		if r.ErrorType != "" {
			msg = fmt.Sprintf("%s: %s", r.ErrorType, msg)
		}
		return nil, fmt.Errorf("prometheus query failed: %s", strings.TrimSpace(msg))
	}
	if r.Data.ResultType != "matrix" {
		return nil, fmt.Errorf("prometheus: unexpected result type %q, want matrix", r.Data.ResultType)
	}

	var events []timeline.Event
	for _, s := range r.Data.Result {
		label := seriesLabel(s.Metric)
		for i := 1; i < len(s.Values); i++ {
			prev, cur := s.Values[i-1].Value, s.Values[i].Value

			above := prev < threshold && cur >= threshold
			below := prev >= threshold && cur < threshold
			if !above && !below {
				continue
			}

			direction := "above"
			if below {
				direction = "below"
			}
			events = append(events, timeline.Event{
				Time:   s.Values[i].Time,
				Source: "prometheus",
				Summary: fmt.Sprintf("%s: %s crossed %s %s (value %s)",
					query, label, direction, formatValue(threshold), formatValue(cur)),
			})
		}
	}

	return events, nil
}

// seriesLabel renders a metric's label set the way Prometheus prints it:
// the metric name followed by its remaining labels in sorted order.
func seriesLabel(metric map[string]string) string {
	name := metric["__name__"]

	pairs := make([]string, 0, len(metric))
	for k, v := range metric {
		if k == "__name__" {
			continue
		}
		pairs = append(pairs, fmt.Sprintf("%s=%q", k, v))
	}
	sort.Strings(pairs)

	if len(pairs) == 0 {
		if name == "" {
			return "{}"
		}
		return name
	}
	return name + "{" + strings.Join(pairs, ",") + "}"
}

// formatValue renders a float without trailing zeros or an exponent for the
// common small magnitudes a threshold and a sample carry.
func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
