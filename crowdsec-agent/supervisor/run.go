package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	crowdsecBin   = "/usr/local/bin/crowdsec"
	cscliBin      = "/usr/local/bin/cscli"
	whitelistsSh  = "/usr/local/lib/crowdsec-supervisor/apply-whitelists.sh"
	healthSources = "/tmp_config/health-sources.jsonl"
)

type runConfig struct {
	lapi       bool
	args       []string
	crowdsec   string
	cscli      string
	whitelists string
	sources    string
	healthAddr string
	stall      int
	check      int
	interval   time.Duration
	jitter     time.Duration
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "crowdsec-supervisor: "+format+"\n", args...)
}

// oneShot reports whether crowdsec was asked to do something other than start
// the daemon (config test, version, help, time-machine replay).
func oneShot(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		switch name {
		case "t", "version", "dsn", "h", "help":
			return true
		}
	}
	return false
}

// configArg returns the -c value the daemon was started with, if any.
func configArg(args []string) []string {
	for i, arg := range args {
		switch {
		case arg == "-c" || arg == "--c":
			if i+1 < len(args) {
				return []string{"-c", args[i+1]}
			}
		case strings.HasPrefix(arg, "-c=") || strings.HasPrefix(arg, "--c="):
			return []string{arg}
		}
	}
	return nil
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == "-"+flag || arg == "--"+flag {
			return true
		}
	}
	return false
}

func modeFromEnv() (lapi bool, err error) {
	switch value := os.Getenv("CROWDSEC_SUPERVISOR_MODE"); value {
	case "", "agent":
		return false, nil
	case "lapi":
		return true, nil
	default:
		return false, fmt.Errorf("CROWDSEC_SUPERVISOR_MODE must be agent or lapi, got %q", value)
	}
}

func envInt(name string, fallback int) (int, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return n, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}
	value = strings.TrimSpace(value)
	if n, err := strconv.Atoi(value); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return d, nil
}

func loadRunConfig(lapi bool, args []string) (runConfig, error) {
	cfg := runConfig{
		lapi: lapi, args: args,
		crowdsec: crowdsecBin, cscli: cscliBin, whitelists: whitelistsSh,
		sources: healthSources, healthAddr: healthAddr,
	}
	if lapi {
		return cfg, nil
	}
	var err error
	if cfg.stall, err = envInt("STALL_SECONDS", 120); err != nil {
		return cfg, err
	}
	if cfg.check, err = envInt("CHECK_SECONDS", 15); err != nil {
		return cfg, err
	}
	if cfg.stall < 0 || cfg.check <= 0 {
		return cfg, errors.New("STALL_SECONDS must be >= 0 and CHECK_SECONDS > 0")
	}
	if cfg.interval, err = envDuration("HUB_REFRESH_INTERVAL", 24*time.Hour); err != nil {
		return cfg, err
	}
	if cfg.jitter, err = envDuration("HUB_REFRESH_JITTER", 5*time.Minute); err != nil {
		return cfg, err
	}
	if cfg.interval <= 0 || cfg.jitter < 0 {
		return cfg, errors.New("HUB_REFRESH_INTERVAL must be > 0 and HUB_REFRESH_JITTER >= 0")
	}
	return cfg, nil
}

// HubState is exposed on /hub and records the scheduled refresh outcome.
type HubState struct {
	mu   sync.Mutex
	data hubSnapshot
}

type hubSnapshot struct {
	StartupUpgrade *time.Time `json:"startup_upgrade"`
	LastSuccess    *time.Time `json:"last_success"`
	LastAttempt    *time.Time `json:"last_attempt"`
	LastError      string     `json:"last_error"`
	Running        bool       `json:"running"`
	NextRun        *time.Time `json:"next_run"`
}

func (h *HubState) Snapshot() hubSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.data
}

func (h *HubState) update(f func(*hubSnapshot)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	f(&h.data)
}

func ptr(t time.Time) *time.Time { t = t.UTC().Truncate(time.Second); return &t }

type supervisor struct {
	cfg     runConfig
	procs   *procs
	signals chan os.Signal
	stop    chan os.Signal
	trigger chan struct{}
	devnull *os.File
	hub     *HubState
}

func newSupervisor(cfg runConfig) (*supervisor, error) {
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	s := &supervisor{
		cfg: cfg, procs: newProcs(),
		signals: make(chan os.Signal, 8),
		stop:    make(chan os.Signal, 8),
		trigger: make(chan struct{}, 1),
		devnull: devnull,
		hub:     &HubState{},
	}
	if err := becomeSubreaper(); err != nil && os.Getpid() != 1 {
		logf("cannot become child subreaper: %v", err)
	}
	signal.Notify(s.signals, syscall.SIGCHLD, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGUSR1)
	go s.dispatch()
	return s, nil
}

// dispatch reaps children on SIGCHLD and hands every other signal on.
func (s *supervisor) dispatch() {
	for sig := range s.signals {
		switch sig {
		case syscall.SIGCHLD:
			s.procs.reap()
		case syscall.SIGUSR1:
			select {
			case s.trigger <- struct{}{}:
			default:
			}
		default:
			s.stop <- sig
		}
	}
}

func (s *supervisor) command(path string, discard bool, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	if discard {
		cmd.Stdout = s.devnull
	}
	cmd.Stderr = os.Stderr
	return cmd
}

// runStep runs one hook to completion; cancelling ctx kills it.
func (s *supervisor) runStep(ctx context.Context, cmd *exec.Cmd) error {
	p, err := s.procs.start(cmd)
	if err != nil {
		return err
	}
	defer p.cmd.Process.Release()
	select {
	case status := <-p.status:
		return statusError(status)
	case <-ctx.Done():
		p.signal(syscall.SIGKILL)
		<-p.status
		return ctx.Err()
	}
}

func (s *supervisor) steps(ctx context.Context, cmds ...*exec.Cmd) error {
	for _, cmd := range cmds {
		if err := s.runStep(ctx, cmd); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(cmd.Args, " "), err)
		}
	}
	return nil
}

// startup runs the agent-run.sh hooks; a termination signal aborts them.
func (s *supervisor) startup() (int, bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.steps(ctx,
			s.command(s.cfg.cscli, true, "hub", "upgrade"),
			s.command("sh", false, s.cfg.whitelists))
	}()
	for {
		select {
		case err := <-done:
			if err != nil {
				logf("startup failed: %v", err)
				return 1, false
			}
			now := time.Now()
			s.hub.update(func(h *hubSnapshot) { h.StartupUpgrade = ptr(now) })
			return 0, true
		case sig := <-s.stop:
			if sig == syscall.SIGHUP {
				continue
			}
			cancel()
			<-done
			return 128 + int(sig.(syscall.Signal)), false
		}
	}
}

func (s *supervisor) refresh(ctx context.Context) error {
	return s.steps(ctx,
		s.command(s.cfg.cscli, true, "hub", "update"),
		s.command(s.cfg.cscli, false, "hub", "upgrade"),
		s.command("sh", false, s.cfg.whitelists),
		s.command(s.cfg.crowdsec, false, append([]string{"-t"}, configArg(s.cfg.args)...)...))
}

func (s *supervisor) delay() time.Duration {
	d := s.cfg.interval
	if s.cfg.jitter > 0 {
		d += time.Duration(rand.Int63n(int64(s.cfg.jitter) + 1))
	}
	return d
}

func (s *supervisor) serveHealth(monitor *Monitor) error {
	listener, err := net.Listen("tcp", s.cfg.healthAddr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           healthHandler{monitor: monitor, hub: s.hub},
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil {
			logf("health server stopped: %v", err)
			os.Exit(1)
		}
	}()
	return nil
}

func (s *supervisor) run() int {
	cfg := s.cfg
	args := cfg.args
	if cfg.lapi {
		if !hasFlag(args, "no-cs") {
			args = append([]string{"-no-cs"}, args...)
		}
	} else {
		sources, err := loadSources(cfg.sources)
		if err != nil {
			logf("cannot load health sources: %v", err)
			return 1
		}
		monitor := NewMonitor(sources, cfg.stall, cfg.check)
		if err := s.serveHealth(monitor); err != nil {
			logf("cannot serve health on %s: %v", cfg.healthAddr, err)
			return 1
		}
		go monitor.Loop(nil)
		if code, ok := s.startup(); !ok {
			return code
		}
	}

	child, err := s.procs.start(s.command(cfg.crowdsec, false, args...))
	if err != nil {
		logf("cannot start crowdsec: %v", err)
		return 1
	}
	logf("started %s (pid %d, mode %s)", cfg.crowdsec, child.pid, map[bool]string{true: "lapi", false: "agent"}[cfg.lapi])

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var timer <-chan time.Time
	schedule := func() {
		if cfg.lapi {
			return
		}
		d := s.delay()
		next := time.Now().Add(d)
		s.hub.update(func(h *hubSnapshot) { h.NextRun = ptr(next) })
		timer = time.After(d)
	}
	schedule()
	refreshing := false
	results := make(chan error, 1)
	begin := func() {
		if cfg.lapi || refreshing {
			return
		}
		refreshing = true
		timer = nil
		now := time.Now()
		s.hub.update(func(h *hubSnapshot) { h.Running, h.LastAttempt, h.NextRun = true, ptr(now), nil })
		logf("hub refresh started")
		go func() { results <- s.refresh(ctx) }()
	}

	for {
		select {
		case status := <-child.status:
			cancel()
			if refreshing {
				<-results
			}
			logf("crowdsec exited: %v", describe(status))
			return exitCode(status)
		case sig := <-s.stop:
			child.signal(sig)
		case <-timer:
			begin()
		case <-s.trigger:
			begin()
		case err := <-results:
			refreshing = false
			now := time.Now()
			if err != nil {
				logf("hub refresh failed, keeping current hub: %v", err)
				s.hub.update(func(h *hubSnapshot) { h.Running, h.LastError = false, err.Error() })
			} else {
				child.signal(syscall.SIGHUP)
				s.hub.update(func(h *hubSnapshot) { h.Running, h.LastError, h.LastSuccess = false, "", ptr(now) })
				logf("hub refresh succeeded at %s; sent SIGHUP to crowdsec", now.UTC().Format(time.RFC3339))
			}
			schedule()
		}
	}
}

func describe(status syscall.WaitStatus) string {
	if err := statusError(status); err != nil {
		return err.Error()
	}
	return "exit status 0"
}

func runCrowdsec(lapi bool, args []string) int {
	if oneShot(args) {
		argv := append([]string{"crowdsec"}, args...)
		err := syscall.Exec(crowdsecBin, argv, os.Environ())
		logf("exec %s: %v", crowdsecBin, err)
		return 127
	}
	cfg, err := loadRunConfig(lapi, args)
	if err != nil {
		logf("%v", err)
		return 2
	}
	s, err := newSupervisor(cfg)
	if err != nil {
		logf("%v", err)
		return 1
	}
	return s.run()
}
