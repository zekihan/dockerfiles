# crowdsec-agent

`ghcr.io/zekihan/crowdsec-agent` is the upstream `crowdsecurity/crowdsec` image,
pinned by digest, plus a static Go supervisor. Nothing from upstream is changed:
the final stage only adds files. The image keeps the upstream entrypoint
(`docker_start.sh`), PATH, and writable root filesystem (`cscli` writes the hub
under `/etc/crowdsec`). It is built for Linux AMD64 and ARM64. `version` follows
the CrowdSec release, and Renovate tracks it together with the `FROM` pin.

It replaces the Python `acquisition-health` sidecar, the hub-update CronJob and
the `/runtime-bin` shims in `homelab/apps/crowdsec` (OPS-361).

## Added files

| Path | Purpose |
| --- | --- |
| `/usr/local/bin/crowdsec-supervisor` | static supervisor (`CGO_ENABLED=0`) |
| `/usr/local/sbin/crowdsec` | symlink to the supervisor, earlier on PATH than `/usr/local/bin/crowdsec` |
| `/usr/local/lib/crowdsec-supervisor/apply-whitelists.sh` | the homelab whitelist script, unchanged |
| `/usr/share/licenses/dockerfiles/` | notices |

The upstream entrypoint ends with `exec crowdsec $ARGS`, which resolves to the
symlink. When the arguments ask for a one-shot action (`-t`, `-version`, `-h`,
`-help` or `-dsn`), the supervisor execs `/usr/local/bin/crowdsec` directly.
Otherwise it supervises the daemon.

## Modes

`crowdsec` (through the symlink) and `crowdsec-supervisor run [--lapi] [--] ARGS`
choose the mode from `CROWDSEC_SUPERVISOR_MODE`: `agent` (the default) or `lapi`.
`--lapi` overrides it.

**lapi** starts `/usr/local/bin/crowdsec -no-cs ARGS` as a child (`-no-cs` is
not added twice). It forwards SIGTERM, SIGINT and SIGHUP and exits with the
child's status. There is no health server and no hub refresh.

**agent** first runs `cscli hub upgrade` (stdout discarded) and then
`apply-whitelists.sh`. Either failing stops startup. It then starts
`/usr/local/bin/crowdsec ARGS` as a child. SIGTERM, SIGINT and SIGHUP are
forwarded, and the supervisor exits with the child's status (128+N if a signal
killed it). It acts as a child subreaper, so orphaned processes are reaped
whether or not it is PID 1.

- **Health** on `:6070`, a port of `acquisition-health.py` with the same logic,
  thresholds, log lines and bodies (`OK` or `VictoriaLogs acquisition
  unhealthy`). Sources come from `/tmp_config/health-sources.jsonl`; counters
  are `cs_victorialogssource_hits_total` from `http://127.0.0.1:6060/metrics`.
  `/self` only requires a sample younger than
  `max(60, sources*10 + 2*CHECK_SECONDS)` seconds, `/ready` uses readiness, and
  any other target uses liveness.
- **Hub refresh** runs every `HUB_REFRESH_INTERVAL` plus a random delay of up to
  `HUB_REFRESH_JITTER`, one at a time. It runs `cscli hub update`,
  `cscli hub upgrade`, `apply-whitelists.sh` and `crowdsec -t` (with the
  daemon's `-c`, if given), then sends SIGHUP to the child. A failure is logged,
  no SIGHUP is sent, and the agent keeps running. Sending SIGUSR1 to the
  supervisor starts a refresh at once. `GET /hub` on `:6070` returns JSON with
  `startup_upgrade`, `last_success`, `last_attempt`, `last_error`, `running` and
  `next_run`. Each success also logs
  `crowdsec-supervisor: hub refresh succeeded at <RFC 3339>`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `CROWDSEC_SUPERVISOR_MODE` | `agent` | `agent` or `lapi` |
| `STALL_SECONDS` | `120` | stall and backend-error threshold |
| `CHECK_SECONDS` | `15` | poll interval |
| `HUB_REFRESH_INTERVAL` | `24h` | Go duration or seconds |
| `HUB_REFRESH_JITTER` | `5m` | Go duration or seconds |

## Init containers

`crowdsec-supervisor prepare-agent` and `crowdsec-supervisor prepare-lapi` are
Go versions of `prepare-agent.sh` and `prepare-lapi.sh`. They use the same paths
(`/state`, `/tmp_config`, `/staging/etc/crowdsec/config.yaml`,
`/connection/connection.yaml`, `/runtime/sources.yaml`, `/credentials/*`),
umask 077, and write files with mode 0600. They validate every input before
writing anything, and exit non-zero with a message on missing or malformed
input:

- credentials need non-null `login` and `password` (plus `url` for LAPI);
- `connection.yaml` is exactly one mapping with only `url`, `headers` and
  `auth`, where `url` matches `^https?://` and `headers`/`auth` are mappings;
- every `sources.yaml` document is a mapping. Its `url`, `headers` and `auth`
  are dropped, and it is deep-merged with the connection (yq `*` semantics). At
  least one document is required;
- `victorialogs-acquis.yaml` is the `---`-separated result, and
  `health-sources.jsonl` holds one compact JSON object per document.

For realistic inputs the output matches the yq scripts byte for byte.

## Build and test

```sh
(cd crowdsec-agent/supervisor && go test ./...)
docker buildx build --platform linux/amd64 --load -t local/crowdsec-agent crowdsec-agent
bash crowdsec-agent/smoke-test.sh local/crowdsec-agent
```

The image build runs `gofmt`, `go vet` and `go test` before cross-compiling. The
final stage has no `RUN`, so the ARM64 image builds without emulation.
