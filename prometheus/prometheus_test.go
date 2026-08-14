package prometheus

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

var (
	windowSince = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	windowUntil = time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)
)

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/query_range.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

// doerFunc adapts a plain function to the Doer interface.
type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestDetectFromFixture(t *testing.T) {
	got, err := detect(readFixture(t), 5.0, "node_load1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	want := []struct {
		time    time.Time
		summary string
	}{
		{
			time.Date(2026, 8, 10, 12, 10, 0, 0, time.UTC),
			`node_load1: node_load1{instance="web-1"} crossed above 5 (value 6.1)`,
		},
		{
			time.Date(2026, 8, 10, 12, 20, 0, 0, time.UTC),
			`node_load1: node_load1{instance="web-1"} crossed below 5 (value 2.4)`,
		},
		{
			time.Date(2026, 8, 10, 12, 15, 0, 0, time.UTC),
			`node_load1: node_load1{instance="web-2"} crossed above 5 (value 5.2)`,
		},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if !got[i].Time.Equal(w.time) {
			t.Errorf("event %d: Time = %v, want %v", i, got[i].Time, w.time)
		}
		if got[i].Source != "prometheus" {
			t.Errorf("event %d: Source = %q, want %q", i, got[i].Source, "prometheus")
		}
		if got[i].Summary != w.summary {
			t.Errorf("event %d: Summary = %q, want %q", i, got[i].Summary, w.summary)
		}
	}
}

func TestDetectIgnoresSampleAboveThresholdWithoutCrossing(t *testing.T) {
	// A single sample already above the threshold is not a crossing: there
	// is no earlier sample below it to cross up from.
	body := []byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
		`{"metric":{"__name__":"node_load1","instance":"web-1"},"values":[[1786363200,"9"],[1786363500,"9.5"]]}]}}`)

	got, err := detect(body, 5.0, "node_load1")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d events, want 0: %+v", len(got), got)
	}
}

func TestDetectReportsQueryError(t *testing.T) {
	body := []byte(`{"status":"error","errorType":"bad_data","error":"invalid parameter"}`)
	_, err := detect(body, 5.0, "node_load1")
	if err == nil {
		t.Fatal("detect: want error for failed query, got nil")
	}
}

func TestDetectRejectsNonMatrixResult(t *testing.T) {
	body := []byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`)
	_, err := detect(body, 5.0, "node_load1")
	if err == nil {
		t.Fatal("detect: want error for non-matrix result, got nil")
	}
}

func TestDetectRejectsMalformedSample(t *testing.T) {
	body := []byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
		`{"metric":{"__name__":"node_load1"},"values":[[1786363200,"not-a-number"]]}]}}`)
	_, err := detect(body, 5.0, "node_load1")
	if err == nil {
		t.Fatal("detect: want error for unparseable value, got nil")
	}
}

func TestSourceEventsQueriesRangeAndDetects(t *testing.T) {
	var gotReq *http.Request
	src := Source{
		URL:       "http://prometheus.example:9090/",
		Query:     "node_load1",
		Threshold: 5.0,
		Step:      5 * time.Minute,
		Client: doerFunc(func(req *http.Request) (*http.Response, error) {
			gotReq = req
			return jsonResponse(http.StatusOK, readFixture(t)), nil
		}),
	}

	events, err := src.Events(windowSince, windowUntil)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}

	if gotReq == nil {
		t.Fatal("Events did not issue a request")
	}
	if got, want := gotReq.URL.Path, "/api/v1/query_range"; got != want {
		t.Errorf("request path = %q, want %q", got, want)
	}

	q := gotReq.URL.Query()
	for _, tc := range []struct{ key, want string }{
		{"query", "node_load1"},
		{"start", "1786363200"},
		{"end", "1786366800"},
		{"step", "300"},
	} {
		if got := q.Get(tc.key); got != tc.want {
			t.Errorf("query param %q = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestSourceEventsDefaultsStep(t *testing.T) {
	var gotReq *http.Request
	src := Source{
		URL:   "http://prometheus.example:9090",
		Query: "node_load1",
		Client: doerFunc(func(req *http.Request) (*http.Response, error) {
			gotReq = req
			return jsonResponse(http.StatusOK, readFixture(t)), nil
		}),
	}

	if _, err := src.Events(windowSince, windowUntil); err != nil {
		t.Fatalf("Events: %v", err)
	}
	if got, want := gotReq.URL.Query().Get("step"), "60"; got != want {
		t.Fatalf("step = %q, want %q (default)", got, want)
	}
}

func TestSourceEventsReportsHTTPError(t *testing.T) {
	src := Source{
		URL:   "http://prometheus.example:9090",
		Query: "node_load1",
		Client: doerFunc(func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusInternalServerError, []byte("boom")), nil
		}),
	}

	_, err := src.Events(windowSince, windowUntil)
	if err == nil {
		t.Fatal("Events: want error for non-200 response, got nil")
	}
}

func TestSeriesLabelDropsMetricName(t *testing.T) {
	got := seriesLabel(map[string]string{"instance": "web-1", "job": "node"})
	want := `{instance="web-1",job="node"}`
	if got != want {
		t.Fatalf("seriesLabel = %q, want %q", got, want)
	}
}

func TestFormatHelpers(t *testing.T) {
	if got, want := formatTime(windowSince), "1786363200"; got != want {
		t.Errorf("formatTime = %q, want %q", got, want)
	}
	if got, want := formatStep(5*time.Minute), "300"; got != want {
		t.Errorf("formatStep = %q, want %q", got, want)
	}
}

func TestSourceName(t *testing.T) {
	if got := (Source{}).Name(); got != "prometheus" {
		t.Fatalf("Name() = %q, want %q", got, "prometheus")
	}
}
