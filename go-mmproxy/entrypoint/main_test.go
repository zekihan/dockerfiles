package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseConfigDefaultsToDisabled(t *testing.T) {
	cfg, err := parseConfig(env(nil))
	if err != nil || cfg.enabled || cfg.table != 123 || cfg.priority != 32765 {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestParseConfigOverrides(t *testing.T) {
	cfg, err := parseConfig(env(map[string]string{envSetup: "true", envTable: "200", envPriority: "100"}))
	if err != nil || !cfg.enabled || cfg.table != 200 || cfg.priority != 100 {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	cfg, err = parseConfig(env(map[string]string{envSetup: "false"}))
	if err != nil || cfg.enabled {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestParseConfigRejectsInvalid(t *testing.T) {
	for _, m := range []map[string]string{
		{envSetup: "maybe"},
		{envTable: "0"},
		{envTable: "254"},
		{envTable: "abc"},
		{envPriority: "0"},
		{envPriority: "32766"},
	} {
		if _, err := parseConfig(env(m)); err == nil {
			t.Errorf("%v accepted", m)
		}
	}
}

func TestMessageEncoding(t *testing.T) {
	m := localRouteMessage(syscall.AF_INET, 123, 1, 7)
	if int(native.Uint32(m)) != len(m) || len(m)%4 != 0 {
		t.Fatalf("bad length %d: %x", len(m), m)
	}
	if native.Uint16(m[4:]) != syscall.RTM_NEWROUTE || native.Uint32(m[8:]) != 7 {
		t.Fatalf("bad header %x", m)
	}
	flags := native.Uint16(m[6:])
	if flags&nlmFReplace == 0 || flags&nlmFExcl != 0 {
		t.Fatalf("route must replace, got flags %#x", flags)
	}
	r := iifRuleMessage(syscall.AF_INET, []byte{127, 0, 0, 1}, 8, "lo", 123, 32765, 8)
	if int(native.Uint32(r)) != len(r) || len(r)%4 != 0 {
		t.Fatalf("bad length %d: %x", len(r), r)
	}
	if flags := native.Uint16(r[6:]); flags&nlmFExcl == 0 {
		t.Fatalf("rule must be exclusive, got flags %#x", flags)
	}
	if r[syscall.NLMSG_HDRLEN+2] != 8 {
		t.Fatalf("src_len not set: %x", r)
	}
}

func TestAckError(t *testing.T) {
	ack := func(code int32, seq uint32) []byte {
		p := make([]byte, 4)
		native.PutUint32(p, uint32(code))
		return message(syscall.NLMSG_ERROR, 0, seq, p)
	}
	if err := ackError(ack(0, 3), 3); err != nil {
		t.Fatal(err)
	}
	if err := ackError(ack(-int32(syscall.EEXIST), 3), 3); err != syscall.EEXIST {
		t.Fatalf("got %v", err)
	}
	if err := ackError(ack(0, 2), 3); err == nil {
		t.Fatal("foreign sequence accepted")
	}
	if err := ackError([]byte{1, 2}, 3); err == nil {
		t.Fatal("short response accepted")
	}
}

// TestRoutingInNetns re-executes this test binary in a fresh user+network
// namespace and repeatedly applies the routing. It is skipped when `unshare`
// or `ip` is unavailable or namespaces are not permitted.
func TestRoutingInNetns(t *testing.T) {
	if os.Getenv("MMPROXY_NETNS_CHILD") != "" {
		t.Skip("child process")
	}
	for _, tool := range []string{"unshare", "ip"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not available")
		}
	}
	if err := exec.Command("unshare", "-rn", "true").Run(); err != nil {
		t.Skip("user/network namespaces not permitted: ", err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"dual-stack", "ipv4-only"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command("unshare", "-rn", self, "-test.run=^TestNetnsChild$", "-test.v")
			cmd.Env = append(os.Environ(), "MMPROXY_NETNS_CHILD="+mode)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}

func TestNetnsChild(t *testing.T) {
	mode := os.Getenv("MMPROXY_NETNS_CHILD")
	if mode == "" {
		t.Skip("runs only inside TestRoutingInNetns")
	}
	run := func(args ...string) string {
		out, err := exec.Command("ip", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("ip %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("link", "set", "lo", "up")
	if mode == "ipv4-only" {
		if err := os.WriteFile("/proc/sys/net/ipv6/conf/lo/disable_ipv6", []byte("1\n"), 0); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := applyRouting(123, 32765); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	checks := []struct {
		args []string
		want string
	}{
		{[]string{"rule"}, "from 127.0.0.1/8 iif lo lookup 123"},
		{[]string{"route", "show", "table", "123"}, "local default dev lo"},
		{[]string{"route", "get", "192.0.2.1", "from", "127.0.0.2"}, "dev lo table 123"},
	}
	for _, c := range checks {
		out := run(c.args...)
		if got := strings.Count(out, c.want); got != 1 {
			t.Errorf("ip %v: %d matches for %q, want 1:\n%s", c.args, got, c.want, out)
		}
	}
	rules6 := run("-6", "rule")
	if mode == "ipv4-only" {
		if strings.Contains(rules6, "lookup 123") {
			t.Errorf("unexpected IPv6 rules:\n%s", rules6)
		}
		if out := run("-6", "route", "show", "table", "all"); strings.Contains(out, "table 123") {
			t.Errorf("unexpected IPv6 routes:\n%s", out)
		}
		return
	}
	if strings.Count(rules6, "from ::1 iif lo lookup 123") != 1 {
		t.Errorf("unexpected IPv6 rules:\n%s", rules6)
	}
	if out := run("-6", "route", "show", "table", "123"); strings.Count(out, "local default dev lo") != 1 {
		t.Errorf("unexpected IPv6 routes:\n%s", out)
	}
}
