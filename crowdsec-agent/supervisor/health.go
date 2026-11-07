package main

// Port of the homelab acquisition-health.py sidecar: compare native source
// counters with recent backend events; quiet sources are healthy.

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	metricsURL     = "http://127.0.0.1:6060/metrics"
	healthAddr     = ":6070"
	unhealthyBody  = "VictoriaLogs acquisition unhealthy"
	counterPrefix  = "cs_victorialogssource_hits_total{"
	maxEventLine   = 1048576
	metricsTimeout = 3 * time.Second
	backendTimeout = 5 * time.Second
)

var acquisType = regexp.MustCompile(`acquis_type="([^"]+)"`)

// Progress tracks one source. Times are wall-clock seconds, as in the Python
// original, so the table tests can use the same literal values.
type Progress struct {
	hasCounter bool
	counter    float64
	changed    float64
	errorSince *float64
}

func NewProgress(now float64) *Progress {
	return &Progress{changed: now}
}

func (p *Progress) observe(counter, now float64) {
	if !p.hasCounter || p.counter != counter {
		p.hasCounter, p.counter, p.changed = true, counter, now
	}
}

func (p *Progress) Check(counter float64, hasNewEvents bool, now, threshold float64, backendOK bool) bool {
	p.observe(counter, now)
	if backendOK {
		p.errorSince = nil
	} else if p.errorSince == nil {
		since := now
		p.errorSince = &since
	}
	if p.errorSince != nil && now-*p.errorSince >= threshold {
		return false
	}
	return !(hasNewEvents && now-p.changed >= threshold)
}

func counters(metrics string) (map[string]float64, error) {
	result := map[string]float64{}
	for _, line := range splitLines(metrics) {
		if !strings.HasPrefix(line, counterPrefix) {
			continue
		}
		match := acquisType.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		i := strings.LastIndex(line, " ")
		if i < 0 {
			return nil, fmt.Errorf("malformed metric line %q", line)
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(line[i+1:]), 64)
		if err != nil {
			return nil, err
		}
		result[match[1]] += value
	}
	return result, nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' })
}

type Source struct {
	URL     string
	Query   string
	Type    string
	Headers map[string]string
	User    string
	Pass    string
	HasAuth bool
}

func loadSources(path string) ([]Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sources []Source
	for n, line := range splitLines(string(data)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		source, err := parseSource([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, n+1, err)
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func parseSource(line []byte) (Source, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return Source{}, err
	}
	var s Source
	var ok bool
	if s.URL, ok = raw["url"].(string); !ok {
		return s, errors.New("url must be a string")
	}
	if s.Query, ok = raw["query"].(string); !ok {
		return s, errors.New("query must be a string")
	}
	labels, _ := raw["labels"].(map[string]any)
	if s.Type, ok = labels["type"].(string); !ok {
		return s, errors.New("labels.type must be a string")
	}
	s.Headers = map[string]string{}
	if value, present := raw["headers"]; present {
		headers, ok := value.(map[string]any)
		if !ok {
			return s, errors.New("headers must be a mapping")
		}
		for key, value := range headers {
			text, err := headerValue(value)
			if err != nil {
				return s, fmt.Errorf("header %s: %w", key, err)
			}
			s.Headers[key] = text
		}
	}
	if value, present := raw["auth"]; present {
		auth, ok := value.(map[string]any)
		if !ok {
			return s, errors.New("auth must be a mapping")
		}
		s.User, _ = auth["username"].(string)
		s.Pass, _ = auth["password"].(string)
		s.HasAuth = s.User != "" || s.Pass != ""
	}
	return s, nil
}

func headerValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		if v {
			return "True", nil
		}
		return "False", nil
	}
	return "", errors.New("must be a scalar")
}

func (s Source) request(client *http.Client, path string, params url.Values) (*http.Response, error) {
	target := strings.TrimRight(s.URL, "/") + path
	if params != nil {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range s.Headers {
		req.Header.Set(key, value)
	}
	if s.HasAuth {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(s.User+":"+s.Pass)))
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

type Monitor struct {
	sources   []Source
	threshold float64
	interval  float64
	metrics   string
	progress  map[string]*Progress
	wallNow   func() float64
	monoNow   func() time.Duration
	client    *http.Client
	logf      func(string)

	mu      sync.Mutex
	live    bool
	ready   bool
	checked time.Duration
}

var monoStart = time.Now()

func monotonic() time.Duration { return time.Since(monoStart) }

func wallSeconds() float64 { return float64(time.Now().UnixNano()) / 1e9 }

func NewMonitor(sources []Source, threshold, interval int) *Monitor {
	m := &Monitor{
		sources:   sources,
		threshold: float64(threshold),
		interval:  float64(interval),
		metrics:   metricsURL,
		progress:  map[string]*Progress{},
		wallNow:   wallSeconds,
		monoNow:   monotonic,
		client:    &http.Client{},
		logf:      func(line string) { fmt.Fprintln(os.Stdout, line) },
		live:      true,
	}
	for _, s := range sources {
		m.progress[s.Type] = NewProgress(m.wallNow())
	}
	m.checked = m.monoNow()
	return m
}

func (m *Monitor) fetchCounters() (map[string]float64, error) {
	client := *m.client
	client.Timeout = metricsTimeout
	resp, err := client.Get(m.metrics)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return counters(string(body))
}

func (m *Monitor) hasEvents(s Source, start string) (bool, error) {
	client := *m.client
	client.Timeout = backendTimeout
	params := url.Values{"query": {s.Query}, "start": {start}, "limit": {"1"}}
	resp, err := s.request(&client, "/select/logsql/query", params)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(io.LimitReader(resp.Body, maxEventLine))
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	return strings.TrimSpace(line) != "", nil
}

func isoTime(seconds float64) string {
	sec := int64(seconds)
	nsec := int64((seconds - float64(sec)) * 1e9)
	t := time.Unix(sec, nsec).UTC().Round(time.Microsecond)
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05-07:00")
	}
	return t.Format("2006-01-02T15:04:05.000000-07:00")
}

func (m *Monitor) Poll() {
	values, err := m.fetchCounters()
	if err != nil {
		m.mu.Lock()
		m.live, m.ready = false, false
		m.checked = m.monoNow()
		m.mu.Unlock()
		return
	}
	healthy := true
	for _, s := range m.sources {
		state := m.progress[s.Type]
		counter := values[s.Type]
		state.observe(counter, m.wallNow())
		// Ignore logs from before the last observed counter advance and allow ingestion lag.
		start := isoTime(state.changed + m.interval)
		events, err := m.hasEvents(s, start)
		ok := state.Check(counter, events, m.wallNow(), m.threshold, err == nil)
		healthy = healthy && ok
	}
	m.mu.Lock()
	if m.live != healthy {
		if healthy {
			m.logf("VictoriaLogs acquisition healthy")
		} else {
			m.logf("VictoriaLogs acquisition stalled or backend unavailable")
		}
	}
	m.live, m.ready = healthy, healthy
	m.checked = m.monoNow()
	m.mu.Unlock()
}

func (m *Monitor) Loop(stop <-chan struct{}) {
	for {
		m.Poll()
		select {
		case <-stop:
			return
		case <-time.After(time.Duration(m.interval * float64(time.Second))):
		}
	}
}

func (m *Monitor) freshness() time.Duration {
	window := float64(len(m.sources))*10 + m.interval*2
	if window < 60 {
		window = 60
	}
	return time.Duration(window * float64(time.Second))
}

// Status reports the probe result for a raw request target, matching the
// Python handler: /self only needs a fresh sample, /ready uses readiness and
// every other target uses liveness.
func (m *Monitor) Status(target string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.monoNow()-m.checked >= m.freshness() {
		return false
	}
	switch target {
	case "/self":
		return true
	case "/ready":
		return m.ready
	}
	return m.live
}

type healthHandler struct {
	monitor *Monitor
	hub     *HubState
}

func (h healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	if r.RequestURI == "/hub" && h.hub != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.hub.Snapshot())
		return
	}
	if h.monitor.Status(r.RequestURI) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "OK")
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = io.WriteString(w, unhealthyBody)
}
