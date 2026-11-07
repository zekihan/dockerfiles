// Command mmproxy-entrypoint optionally prepares policy routing for go-mmproxy in
// the current network namespace, then replaces itself with go-mmproxy. Without
// MMPROXY_SETUP_ROUTING it only execs go-mmproxy with the given arguments.
package main

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
)

const (
	envSetup    = "MMPROXY_SETUP_ROUTING"
	envTable    = "MMPROXY_ROUTING_TABLE"
	envPriority = "MMPROXY_ROUTING_PRIORITY"

	defaultTable    = 123
	defaultPriority = 32765

	mmproxyBinary = "/usr/local/bin/go-mmproxy"
)

type routingConfig struct {
	enabled         bool
	table, priority uint32
}

func parseConfig(getenv func(string) string) (routingConfig, error) {
	cfg := routingConfig{table: defaultTable, priority: defaultPriority}
	if v := getenv(envSetup); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("%s: %w", envSetup, err)
		}
		cfg.enabled = enabled
	}
	for _, e := range []struct {
		name string
		dst  *uint32
		min  uint64
		max  uint64
	}{
		// Tables 253-255 are the kernel's default, main and local tables.
		{envTable, &cfg.table, 1, 0xfffffffe},
		{envPriority, &cfg.priority, 1, 32765},
	} {
		v := getenv(e.name)
		if v == "" {
			continue
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || n < e.min || n > e.max {
			return cfg, fmt.Errorf("%s: %q is not an integer in %d..%d", e.name, v, e.min, e.max)
		}
		*e.dst = uint32(n)
	}
	if cfg.table >= 253 && cfg.table <= 255 {
		return cfg, fmt.Errorf("%s: table %d is reserved", envTable, cfg.table)
	}
	return cfg, nil
}

func main() {
	cfg, err := parseConfig(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mmproxy-entrypoint:", err)
		os.Exit(2)
	}
	if cfg.enabled {
		if err := applyRouting(cfg.table, cfg.priority); err != nil {
			fmt.Fprintln(os.Stderr, "mmproxy-entrypoint: routing setup failed (needs NET_ADMIN):", err)
			os.Exit(1)
		}
	}
	args := append([]string{mmproxyBinary}, os.Args[1:]...)
	err = syscall.Exec(mmproxyBinary, args, os.Environ())
	fmt.Fprintln(os.Stderr, "mmproxy-entrypoint: exec", mmproxyBinary+":", err)
	os.Exit(127)
}
