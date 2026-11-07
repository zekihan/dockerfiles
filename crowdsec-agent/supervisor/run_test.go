package main

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestOneShot(t *testing.T) {
	cases := map[string]bool{
		"":                                false,
		"-c /etc/crowdsec/config.yaml":    false,
		"-c /etc/crowdsec/config.yaml -t": true,
		"-t":                              true,
		"--t":                             true,
		"-version":                        true,
		"-h":                              true,
		"-dsn file:///x -type syslog":     true,
		"-dsn=file:///x":                  true,
		"-no-cs -info":                    false,
		"-c test.yaml -debug":             false,
		"-- -t":                           false,
	}
	for args, want := range cases {
		if got := oneShot(strings.Fields(args)); got != want {
			t.Errorf("oneShot(%q) = %v", args, got)
		}
	}
}

func TestConfigArg(t *testing.T) {
	cases := map[string][]string{
		"":                     nil,
		"-c /x.yaml -info":     {"-c", "/x.yaml"},
		"-info --c=/y.yaml":    {"--c=/y.yaml"},
		"-c":                   nil,
		"-no-cs -c=/z.yaml -t": {"-c=/z.yaml"},
	}
	for args, want := range cases {
		if got := configArg(strings.Fields(args)); !reflect.DeepEqual(got, want) {
			t.Errorf("configArg(%q) = %q", args, got)
		}
	}
}

func TestParseRun(t *testing.T) {
	cases := []struct {
		env  string
		args string
		lapi bool
		rest []string
		err  bool
	}{
		{"", "", false, nil, false},
		{"", "--lapi -- -c x", true, []string{"-c", "x"}, false},
		{"lapi", "-c x", true, []string{"-c", "x"}, false},
		{"lapi", "--agent --", false, []string{}, false},
		{"agent", "--lapi -info", true, []string{"-info"}, false},
		{"bogus", "", false, nil, true},
	}
	for _, tc := range cases {
		t.Setenv("CROWDSEC_SUPERVISOR_MODE", tc.env)
		lapi, rest, err := parseRun(strings.Fields(tc.args))
		if (err != nil) != tc.err || lapi != tc.lapi || (!tc.err && len(rest)+len(tc.rest) > 0 && !reflect.DeepEqual(rest, tc.rest)) {
			t.Errorf("parseRun(%q, env %q) = %v %q %v", tc.args, tc.env, lapi, rest, err)
		}
	}
}

func TestLoadRunConfig(t *testing.T) {
	cfg, err := loadRunConfig(false, nil)
	if err != nil || cfg.stall != 120 || cfg.check != 15 || cfg.interval != 24*time.Hour || cfg.jitter != 5*time.Minute {
		t.Fatalf("defaults %+v %v", cfg, err)
	}
	t.Setenv("HUB_REFRESH_INTERVAL", "90")
	t.Setenv("HUB_REFRESH_JITTER", "0s")
	t.Setenv("STALL_SECONDS", "5")
	t.Setenv("CHECK_SECONDS", "1")
	cfg, err = loadRunConfig(false, nil)
	if err != nil || cfg.stall != 5 || cfg.check != 1 || cfg.interval != 90*time.Second || cfg.jitter != 0 {
		t.Fatalf("overrides %+v %v", cfg, err)
	}
	for name, value := range map[string]string{"HUB_REFRESH_INTERVAL": "0", "CHECK_SECONDS": "x", "HUB_REFRESH_JITTER": "-1s"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := loadRunConfig(false, nil); err == nil {
				t.Fatalf("%s=%s accepted", name, value)
			}
		})
	}
}

const fakeCrowdsec = `#!/bin/sh
if [ "$1" = -t ]; then
  echo "test $*" >> "$TRACE"
  exit "${TEST_EXIT:-0}"
fi
trap 'echo hup >> "$TRACE"' HUP
trap 'echo term >> "$TRACE"; exit 3' TERM
trap 'exit 130' INT
echo "engine $*" >> "$TRACE"
while :; do sleep 0.05; done
`

const fakeCscli = `#!/bin/sh
echo "cscli $*" >> "$TRACE"
echo "cscli stdout $*"
case " $CSCLI_FAIL " in *" $2 "*) exit 1 ;; esac
`

type harness struct {
	t     *testing.T
	trace string
	s     *supervisor
	code  chan int
}

func newHarness(t *testing.T, lapi bool, args ...string) *harness {
	dir := t.TempDir()
	trace := filepath.Join(dir, "trace")
	t.Setenv("TRACE", trace)
	for name, body := range map[string]string{
		"crowdsec":            fakeCrowdsec,
		"cscli":               fakeCscli,
		"apply-whitelists.sh": `echo whitelists >> "$TRACE"` + "\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sources := filepath.Join(dir, "health-sources.jsonl")
	if err := os.WriteFile(sources, []byte(`{"url":"http://127.0.0.1:1/","query":"*","labels":{"type":"traefik"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := runConfig{
		lapi: lapi, args: args,
		crowdsec: filepath.Join(dir, "crowdsec"), cscli: filepath.Join(dir, "cscli"),
		whitelists: filepath.Join(dir, "apply-whitelists.sh"), sources: sources,
		healthAddr: "127.0.0.1:0", stall: 120, check: 15,
		interval: time.Hour, jitter: 0,
	}
	s, err := newSupervisor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { signal.Stop(s.signals) })
	h := &harness{t: t, trace: trace, s: s, code: make(chan int, 1)}
	go func() { h.code <- s.run() }()
	return h
}

func (h *harness) lines() []string {
	data, _ := os.ReadFile(h.trace)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return lines
}

func (h *harness) waitFor(want ...string) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if reflect.DeepEqual(h.lines(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("trace %q, want %q", h.lines(), want)
}

func (h *harness) exit() int {
	h.t.Helper()
	select {
	case code := <-h.code:
		return code
	case <-time.After(10 * time.Second):
		h.t.Fatal("supervisor did not exit")
	}
	return -1
}

func TestAgentStartupRefreshAndShutdown(t *testing.T) {
	h := newHarness(t, false, "-c", "/etc/crowdsec/config.yaml", "-info")
	start := []string{"cscli hub upgrade", "whitelists", "engine -c /etc/crowdsec/config.yaml -info"}
	h.waitFor(start...)
	if snap := h.s.hub.Snapshot(); snap.StartupUpgrade == nil || snap.LastSuccess != nil || snap.NextRun == nil {
		t.Fatalf("hub state after startup %+v", snap)
	}

	h.s.trigger <- struct{}{}
	refresh := append(start, "cscli hub update", "cscli hub upgrade", "whitelists", "test -t -c /etc/crowdsec/config.yaml", "hup")
	h.waitFor(refresh...)
	if snap := h.s.hub.Snapshot(); snap.LastSuccess == nil || snap.Running || snap.LastError != "" {
		t.Fatalf("hub state after refresh %+v", snap)
	}

	t.Setenv("CSCLI_FAIL", "upgrade")
	h.s.trigger <- struct{}{}
	failed := append(refresh, "cscli hub update", "cscli hub upgrade")
	h.waitFor(failed...)
	deadline := time.Now().Add(5 * time.Second)
	for h.s.hub.Snapshot().LastError == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if snap := h.s.hub.Snapshot(); !strings.Contains(snap.LastError, "hub upgrade") {
		t.Fatalf("hub state after failure %+v", snap)
	}
	time.Sleep(200 * time.Millisecond)
	h.waitFor(failed...) // no SIGHUP after a failed refresh

	h.s.stop <- syscall.SIGTERM
	if code := h.exit(); code != 3 {
		t.Fatalf("exit %d, want the child's 3", code)
	}
	h.waitFor(append(failed, "term")...)
}

func TestAgentStartupFailureStopsBeforeEngine(t *testing.T) {
	t.Setenv("CSCLI_FAIL", "upgrade")
	h := newHarness(t, false)
	if code := h.exit(); code == 0 {
		t.Fatal("startup failure exited 0")
	}
	h.waitFor("cscli hub upgrade")
}

func TestAgentRefreshConfigTestFailureKeepsRunning(t *testing.T) {
	t.Setenv("TEST_EXIT", "1")
	h := newHarness(t, false)
	h.waitFor("cscli hub upgrade", "whitelists", "engine")
	h.s.trigger <- struct{}{}
	want := []string{"cscli hub upgrade", "whitelists", "engine", "cscli hub update", "cscli hub upgrade", "whitelists", "test -t"}
	h.waitFor(want...)
	time.Sleep(200 * time.Millisecond)
	h.waitFor(want...)
	h.s.stop <- syscall.SIGINT
	if code := h.exit(); code != 130 {
		t.Fatalf("exit %d, want 130 for SIGINT", code)
	}
}

func TestLAPIForwardsSignals(t *testing.T) {
	h := newHarness(t, true, "-info")
	h.waitFor("engine -no-cs -info")
	h.s.stop <- syscall.SIGHUP
	h.waitFor("engine -no-cs -info", "hup")
	h.s.trigger <- struct{}{} // no hub refresh in LAPI mode
	time.Sleep(200 * time.Millisecond)
	h.waitFor("engine -no-cs -info", "hup")
	h.s.stop <- syscall.SIGTERM
	if code := h.exit(); code != 3 {
		t.Fatalf("exit %d", code)
	}
}

func TestLAPIDoesNotDuplicateNoCS(t *testing.T) {
	h := newHarness(t, true, "-no-cs")
	h.waitFor("engine -no-cs")
	h.s.stop <- syscall.SIGTERM
	h.exit()
}

func TestReapsOrphans(t *testing.T) {
	if os.Getpid() != 1 {
		if err := becomeSubreaper(); err != nil {
			t.Skip("cannot become subreaper:", err)
		}
	}
	p := newProcs()
	// The shell exits at once, orphaning a short sleep that reparents to us.
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command("sh", "-c", `sleep 0.2 & echo $! > "$1"; exit 0`, "sh", pidFile)
	pr, err := p.start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var status syscall.WaitStatus
	got := false
	for !got && time.Now().Before(deadline) {
		p.reap()
		select {
		case status = <-pr.status:
			got = true
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !got || exitCode(status) != 0 {
		t.Fatalf("tracked child status %v %v", got, status)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	orphan, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if pid, err := syscall.Wait4(orphan, nil, syscall.WNOHANG, nil); pid != 0 || err != nil {
		t.Fatalf("orphan %d was not reparented to the subreaper: %d %v", orphan, pid, err)
	}
	time.Sleep(400 * time.Millisecond)
	p.reap()
	if _, err := syscall.Wait4(orphan, nil, syscall.WNOHANG, nil); err != syscall.ECHILD {
		t.Fatalf("orphan left unreaped: %v", err)
	}
}
