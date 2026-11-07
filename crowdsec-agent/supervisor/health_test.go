package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type check struct {
	counter   float64
	events    bool
	now       float64
	backendOK bool
	want      bool
}

func TestProgress(t *testing.T) {
	cases := []struct {
		name   string
		checks []check
	}{
		{"stall and quiet", []check{
			{10, false, 0, true, true},
			{10, false, 180, true, true},
			{10, true, 181, true, false},
			{11, true, 182, true, true},
			{0, false, 183, true, true},
		}},
		{"backend errors and recovery", []check{
			{0, false, 0, false, true},
			{0, false, 121, false, false},
			{0, false, 122, true, true},
		}},
		{"backend error resets after success", []check{
			{0, false, 0, false, true},
			{0, false, 100, true, true},
			{0, false, 150, false, true},
			{0, false, 269, false, true},
			{0, false, 270, false, false},
		}},
		{"events while counter advances stay healthy", []check{
			{1, true, 0, true, true},
			{2, true, 119, true, true},
			{3, true, 238, true, true},
			{3, true, 357, true, true},
			{3, true, 358, true, false},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := NewProgress(0)
			for i, c := range tc.checks {
				if got := state.Check(c.counter, c.events, c.now, 120, c.backendOK); got != c.want {
					t.Fatalf("check %d (%+v) = %v", i, c, got)
				}
			}
		})
	}
}

func TestCounters(t *testing.T) {
	cases := []struct {
		name    string
		metrics string
		want    map[string]float64
		err     bool
	}{
		{"sources summed by type", strings.Join([]string{
			`cs_victorialogssource_hits_total{source="http://test",datasource_type="victorialogs",acquis_type="traefik"} 14`,
			`cs_victorialogssource_hits_total{source="http://test",datasource_type="victorialogs",acquis_type="sshd"} 2`,
			`cs_victorialogssource_hits_total{source="http://other",datasource_type="victorialogs",acquis_type="sshd"} 3`,
			`cs_parser_hits_total{parser="test"} 30`,
			`# TYPE cs_victorialogssource_hits_total counter`,
		}, "\n"), map[string]float64{"traefik": 14, "sshd": 5}, false},
		{"no sources", "cs_parser_hits_total{} 1\n", map[string]float64{}, false},
		{"crlf and float", "cs_victorialogssource_hits_total{acquis_type=\"a\"} 1.5e1\r\n", map[string]float64{"a": 15}, false},
		{"bad value", `cs_victorialogssource_hits_total{acquis_type="a"} x`, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := counters(tc.metrics)
			if (err != nil) != tc.err {
				t.Fatalf("err = %v", err)
			}
			if !tc.err && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsoTime(t *testing.T) {
	cases := map[float64]string{
		0:                 "1970-01-01T00:00:00+00:00",
		1762516815.25:     "2025-11-07T12:00:15.250000+00:00",
		1762516815.000001: "2025-11-07T12:00:15.000001+00:00",
	}
	for in, want := range cases {
		if got := isoTime(in); got != want {
			t.Errorf("isoTime(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSource(t *testing.T) {
	line := `{"source":"victorialogs","url":"http://vl/","query":"q","labels":{"type":"traefik"},"headers":{"AccountID":"0","ProjectID":7},"auth":{"username":"u","password":"p"}}`
	s, err := parseSource([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if s.Type != "traefik" || s.Headers["ProjectID"] != "7" || s.Headers["AccountID"] != "0" || !s.HasAuth {
		t.Fatalf("unexpected %+v", s)
	}
	for _, bad := range []string{
		`{"url":"http://vl/","query":"q"}`,
		`{"url":"http://vl/","labels":{"type":"x"}}`,
		`{"query":"q","labels":{"type":"x"}}`,
		`{"url":"http://vl/","query":"q","labels":{"type":"x"},"headers":"nope"}`,
		`not json`,
	} {
		if _, err := parseSource([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestLoadSourcesSkipsBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health-sources.jsonl")
	data := `{"url":"http://a/","query":"q","labels":{"type":"a"}}` + "\n\n" +
		`{"url":"http://b/","query":"q","labels":{"type":"b"}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	sources, err := loadSources(path)
	if err != nil || len(sources) != 2 || sources[1].Type != "b" {
		t.Fatalf("sources %+v, err %v", sources, err)
	}
}

// fakeEnv simulates crowdsec metrics and a VictoriaLogs backend.
type fakeEnv struct {
	mu        sync.Mutex
	counter   float64
	events    bool
	backend   int
	metricsOK bool
	requests  []*http.Request
}

func (f *fakeEnv) servers(t *testing.T) (metrics, backend *httptest.Server) {
	metrics = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.metricsOK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "cs_victorialogssource_hits_total{acquis_type=\"traefik\"} %v\n", f.counter)
	}))
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r)
		w.WriteHeader(f.backend)
		if f.events {
			io.WriteString(w, `{"_msg":"hit"}`+"\n"+`{"_msg":"more"}`+"\n")
		}
	}))
	t.Cleanup(metrics.Close)
	t.Cleanup(backend.Close)
	return
}

type clock struct{ wall, mono float64 }

func newTestMonitor(t *testing.T, f *fakeEnv, sources int) (*Monitor, *clock, *[]string) {
	metrics, backend := f.servers(t)
	var list []Source
	for i := 0; i < sources; i++ {
		list = append(list, Source{
			URL: backend.URL + "/", Query: `kubernetes.container_name:="traefik"`, Type: "traefik",
			Headers: map[string]string{"AccountID": "0"}, User: "user", Pass: "secret", HasAuth: true,
		})
	}
	c := &clock{wall: 1000}
	m := NewMonitor(list, 120, 15)
	m.metrics = metrics.URL + "/metrics"
	m.wallNow = func() float64 { return c.wall }
	m.monoNow = func() time.Duration { return time.Duration(c.mono * float64(time.Second)) }
	m.checked = m.monoNow()
	var logs []string
	m.logf = func(line string) { logs = append(logs, line) }
	return m, c, &logs
}

func probe(t *testing.T, m *Monitor, target string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	healthHandler{monitor: m, hub: &HubState{}}.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestMonitorScenarios(t *testing.T) {
	type step struct {
		advance   float64 // seconds of wall and monotonic time before polling
		counter   float64
		events    bool
		backend   int
		metricsOK bool
		poll      bool
		live      int
		ready     int
		self      int
	}
	ok, bad := http.StatusOK, http.StatusServiceUnavailable
	cases := []struct {
		name  string
		steps []step
		logs  []string
	}{
		{"initially live but not ready", []step{
			{0, 0, false, 200, true, false, ok, bad, ok},
		}, nil},
		{"counter progress keeps events healthy", []step{
			{0, 1, true, 200, true, true, ok, ok, ok},
			{15, 2, true, 200, true, true, ok, ok, ok},
			{110, 3, true, 200, true, true, ok, ok, ok},
			{110, 4, true, 200, true, true, ok, ok, ok},
		}, nil},
		{"stalled counter with backend events fails then recovers", []step{
			{0, 5, true, 200, true, true, ok, ok, ok},
			{60, 5, true, 200, true, true, ok, ok, ok},
			{60, 5, true, 200, true, true, bad, bad, ok},
			{15, 6, true, 200, true, true, ok, ok, ok},
		}, []string{"VictoriaLogs acquisition stalled or backend unavailable", "VictoriaLogs acquisition healthy"}},
		{"quiet source stays healthy", []step{
			{0, 5, false, 200, true, true, ok, ok, ok},
			{300, 5, false, 200, true, true, ok, ok, ok},
		}, nil},
		{"backend errors fail after threshold", []step{
			{0, 5, false, 500, true, true, ok, ok, ok},
			{119, 5, false, 500, true, true, ok, ok, ok},
			{1, 5, false, 500, true, true, bad, bad, ok},
			{1, 5, false, 200, true, true, ok, ok, ok},
		}, []string{"VictoriaLogs acquisition stalled or backend unavailable", "VictoriaLogs acquisition healthy"}},
		{"metrics failure is immediately unhealthy without a log line", []step{
			{0, 5, false, 200, false, true, bad, bad, ok},
			{15, 5, false, 200, true, true, ok, ok, ok},
		}, []string{"VictoriaLogs acquisition healthy"}},
		{"stale sample fails every probe including self", []step{
			{0, 5, false, 200, true, true, ok, ok, ok},
			{59, 5, false, 200, true, false, ok, ok, ok},
			{1, 5, false, 200, true, false, bad, bad, bad},
			{0, 5, false, 200, true, true, ok, ok, ok},
		}, nil},
		{"stale before first poll", []step{
			{60, 0, false, 200, true, false, bad, bad, bad},
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeEnv{}
			m, c, logs := newTestMonitor(t, f, 1)
			for i, s := range tc.steps {
				c.wall += s.advance
				c.mono += s.advance
				f.mu.Lock()
				f.counter, f.events, f.backend, f.metricsOK = s.counter, s.events, s.backend, s.metricsOK
				f.mu.Unlock()
				if s.poll {
					m.Poll()
				}
				for target, want := range map[string]int{"/live": s.live, "/ready": s.ready, "/self": s.self} {
					code, body := probe(t, m, target)
					wantBody := "OK"
					if want != ok {
						wantBody = unhealthyBody
					}
					if code != want || body != wantBody {
						t.Fatalf("step %d %s = %d %q, want %d", i, target, code, body, want)
					}
				}
			}
			if !reflect.DeepEqual(*logs, tc.logs) {
				t.Fatalf("logs %q, want %q", *logs, tc.logs)
			}
		})
	}
}

func TestFreshnessWindowScalesWithSources(t *testing.T) {
	cases := []struct {
		sources  int
		interval int
		want     time.Duration
	}{
		{1, 15, 60 * time.Second},
		{2, 25, 70 * time.Second},
		{6, 15, 90 * time.Second},
	}
	for _, tc := range cases {
		m := NewMonitor(make([]Source, tc.sources), 120, tc.interval)
		if got := m.freshness(); got != tc.want {
			t.Errorf("%d sources, interval %d: %v, want %v", tc.sources, tc.interval, got, tc.want)
		}
	}
}

func TestBackendRequest(t *testing.T) {
	f := &fakeEnv{metricsOK: true, backend: 200, counter: 3}
	m, c, _ := newTestMonitor(t, f, 1)
	m.Poll()
	if len(f.requests) != 1 {
		t.Fatalf("%d requests", len(f.requests))
	}
	r := f.requests[0]
	if r.URL.Path != "/select/logsql/query" {
		t.Errorf("path %s", r.URL.Path)
	}
	want := url.Values{"query": {`kubernetes.container_name:="traefik"`}, "start": {isoTime(c.wall + 15)}, "limit": {"1"}}
	if !reflect.DeepEqual(r.URL.Query(), want) {
		t.Errorf("query %v, want %v", r.URL.Query(), want)
	}
	if user, pass, ok := r.BasicAuth(); !ok || user != "user" || pass != "secret" {
		t.Errorf("auth %q %q %v", user, pass, ok)
	}
	if r.Header.Get("AccountID") != "0" {
		t.Errorf("headers %v", r.Header)
	}
}

func TestHandlerTargets(t *testing.T) {
	f := &fakeEnv{metricsOK: true, backend: 200}
	m, _, _ := newTestMonitor(t, f, 1)
	m.live, m.ready = true, false
	for target, want := range map[string]int{
		"/live": 200, "/ready": 503, "/self": 200, "/": 200, "/ready?x=1": 200, "/anything": 200,
	} {
		if code, _ := probe(t, m, target); code != want {
			t.Errorf("%s = %d, want %d", target, code, want)
		}
	}
	rec := httptest.NewRecorder()
	healthHandler{monitor: m}.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/live", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("HEAD = %d", rec.Code)
	}
	hub := &HubState{}
	now := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	hub.update(func(h *hubSnapshot) { h.LastSuccess = &now })
	rec = httptest.NewRecorder()
	healthHandler{monitor: m, hub: hub}.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/hub", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"last_success":"2026-11-07T00:00:00Z"`) {
		t.Errorf("/hub = %d %s", rec.Code, rec.Body.String())
	}
}
