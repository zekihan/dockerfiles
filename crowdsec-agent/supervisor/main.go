// Command crowdsec-supervisor runs CrowdSec with acquisition health checks and
// scheduled hub refreshes, and prepares agent and LAPI state in init containers.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const usage = `usage:
  crowdsec-supervisor run [--lapi] [--] [crowdsec args...]
  crowdsec-supervisor prepare-agent
  crowdsec-supervisor prepare-lapi

Invoked as "crowdsec" (via the /usr/local/sbin/crowdsec symlink) it behaves
like "run", choosing the mode from CROWDSEC_SUPERVISOR_MODE (agent or lapi).
`

func main() {
	os.Exit(dispatch(filepath.Base(os.Args[0]), os.Args[1:]))
}

func dispatch(name string, args []string) int {
	if name == "crowdsec" {
		lapi, err := modeFromEnv()
		if err != nil {
			logf("%v", err)
			return 2
		}
		return runCrowdsec(lapi, args)
	}
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		lapi, rest, err := parseRun(args[1:])
		if err != nil {
			logf("%v", err)
			return 2
		}
		return runCrowdsec(lapi, rest)
	case "prepare-agent":
		return runPrepare(args[0], prepareAgent)
	case "prepare-lapi":
		return runPrepare(args[0], prepareLAPI)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
		return 0
	}
	fmt.Fprintf(os.Stderr, "unknown command %q\n%s", args[0], usage)
	return 2
}

func parseRun(args []string) (bool, []string, error) {
	lapi, err := modeFromEnv()
	if err != nil {
		return false, nil, err
	}
	for i, arg := range args {
		switch arg {
		case "--lapi":
			lapi = true
		case "--agent":
			lapi = false
		case "--":
			return lapi, args[i+1:], nil
		default:
			return lapi, args[i:], nil
		}
	}
	return lapi, nil, nil
}
