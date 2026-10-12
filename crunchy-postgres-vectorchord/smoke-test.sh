#!/usr/bin/env bash
set -euo pipefail

image=${1:?Usage: smoke-test.sh IMAGE}
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
expected_version=$(awk '$1 == "version:" { print $2 }' "$script_dir/config.yaml")
container=$(docker create -i --platform linux/amd64 --user 26:26 \
    --env EXPECTED_VERSION="$expected_version" --tmpfs /tmp:rw,mode=1777 --entrypoint /bin/bash "$image" -s)
cleanup() {
    docker rm -f "$container" >/dev/null
}
trap cleanup EXIT
# Copy the test so the same command works with local and remote Docker engines.
docker cp "$script_dir/smoke-test.sql" "$container:/smoke-test.sql"
docker start -ai "$container" <<'CONTAINER'
set -euo pipefail
export PGDATA=/tmp/pgdata PGHOST=/tmp
initdb --auth-local=trust --auth-host=reject --no-locale >/dev/null
pg_ctl -w -l /tmp/postgres.log -o "-c listen_addresses='' -c unix_socket_directories=/tmp -c shared_preload_libraries=pg_stat_statements,vchord.so" start || { cat /tmp/postgres.log; exit 1; }
cleanup_database() {
    result=$?
    if [ "$result" -ne 0 ]; then cat /tmp/postgres.log; fi
    pg_ctl -m immediate -w stop >/dev/null
    exit "$result"
}
trap cleanup_database EXIT
psql -X -v ON_ERROR_STOP=1 -d postgres -f /smoke-test.sql
actual_version="$(pg_config --version | cut -d ' ' -f 2)-vectorchord$(psql -XAt -d postgres -c "SELECT extversion FROM pg_extension WHERE extname='vchord';")"
test "$actual_version" = "$EXPECTED_VERSION"
pg_dump -Fc -d postgres -f /tmp/backup.dump
createdb restored
pg_restore --exit-on-error -d restored /tmp/backup.dump
test "$(psql -XAt -v ON_ERROR_STOP=1 -d restored -c 'SELECT count(*) FROM embeddings;')" = 1000
psql -X -v ON_ERROR_STOP=1 -d restored -c "SET enable_seqscan=off; SET vchordrq.probes='1'; SELECT id FROM embeddings ORDER BY embedding <-> '[500,1,0]' LIMIT 1;"
pg_ctl -m fast -w restart -l /tmp/postgres.log
psql -X -v ON_ERROR_STOP=1 -d postgres -f /smoke-test.sql
CONTAINER
status=$(docker inspect --format '{{.State.ExitCode}}' "$container")
test "$status" = 0
