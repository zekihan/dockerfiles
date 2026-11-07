#!/usr/bin/env bash
# Smoke-test a prepared crowdsec-agent image without network or credentials.
set -euo pipefail

image=${1:?usage: smoke-test.sh IMAGE}
work=$(mktemp -d)
trap 'docker run --rm -v "$work:/work" --entrypoint rm "$image" -rf /work/state /work/tmp_config; rm -rf "$work"' EXIT

docker run --rm --entrypoint sh "$image" -c '
  set -eu
  test "$(command -v crowdsec)" = /usr/local/sbin/crowdsec
  test "$(readlink /usr/local/sbin/crowdsec)" = /usr/local/bin/crowdsec-supervisor
  test -s /usr/local/lib/crowdsec-supervisor/apply-whitelists.sh
  test -s /usr/share/licenses/dockerfiles/NOTICE.md
  command -v yq cscli >/dev/null
'
docker run --rm --entrypoint crowdsec "$image" -version | grep -q '^version: v'

mkdir -p "$work/tmp_config" "$work/connection" "$work/runtime"
printf 'url: http://lapi:8080/\nlogin: smoke\npassword: disposable\n' > "$work/tmp_config/local_api_credentials.yaml"
printf 'url: http://victorialogs:9428/\nheaders:\n  AccountID: "0"\n' > "$work/connection/connection.yaml"
printf -- '---\nsource: victorialogs\nmode: tail\nurl: http://configured-by-eso.invalid/\nquery: "*"\nlabels:\n  type: traefik\n' > "$work/runtime/sources.yaml"
run_prepare() {
  docker run --rm -v "$work/state:/state" -v "$work/tmp_config:/tmp_config" \
    -v "$work/connection:/connection:ro" -v "$work/runtime:/runtime:ro" \
    --entrypoint crowdsec-supervisor "$image" prepare-agent
}
run_prepare
docker run --rm -v "$work:/work:ro" --entrypoint sh "$image" -c '
  set -eu
  test "$(stat -c %a /work/state/local_api_credentials.yaml)" = 600
  test "$(stat -c %a /work/tmp_config/victorialogs-acquis.yaml)" = 600
  test "$(yq ".url" /work/tmp_config/victorialogs-acquis.yaml)" = http://victorialogs:9428/
  test "$(yq -p=json -o=yaml ".headers.AccountID" /work/tmp_config/health-sources.jsonl)" = 0
  test -s /work/state/config.yaml
'
printf 'url: http://victorialogs:9428/\nquery: "*"\n' > "$work/connection/connection.yaml"
if run_prepare 2>/dev/null; then
  echo "prepare-agent accepted a connection with unexpected keys" >&2
  exit 1
fi
echo "crowdsec-agent smoke test passed"
