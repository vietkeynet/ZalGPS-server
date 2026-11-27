#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Local synthetic PostgreSQL integration test; never connects to an existing DB.
set -euo pipefail
umask 077
die() { printf '%s\n' "$*" >&2; exit 1; }
[[ $(id -u) != 0 ]] || die "Run as an unprivileged user."
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
for tool in go pg_config mktemp timeout df awk; do
    command -v "$tool" >/dev/null || die "Missing prerequisite: $tool"
done
pgbin=$(pg_config --bindir)
for tool in initdb pg_ctl postgres; do
    [[ -x "$pgbin/$tool" ]] || die "Missing PostgreSQL tool: $tool"
done
[[ $("$pgbin/postgres" --version) =~ PostgreSQL\)\ 16\. ]] || die "PostgreSQL 16 is required for this verified test profile."
base=${ZALGPS_TEST_TMPDIR:-/tmp}
[[ -d "$base" && -w "$base" ]] || die "Test scratch directory must be writable."
base=$(cd -- "$base" && pwd -P)
[[ $base =~ ^/[a-zA-Z0-9_./-]+$ ]] || die "Use an absolute scratch directory without whitespace or special characters."
available=$(df -Pk "$base" | awk 'NR==2{print $4}')
[[ $available =~ ^[0-9]+$ ]] && (( available >= 2097152 )) || die "At least 2 GiB free scratch space required."
scratch=$(mktemp -d "$base/zalgps-test.XXXXXXXX")
cleanup() {
    rc=$?
    trap - EXIT
    # pg_ctl may return failure after starting postgres; always check status.
    if [[ -f "$scratch/db/postmaster.pid" ]]; then
        if ! "$pgbin/pg_ctl" -D "$scratch/db" -m fast -w -t 30 stop >/dev/null 2>&1; then
            printf 'STOP FAILED; private scratch retained: %s\n' "$scratch" >&2
            exit 1
        fi
    fi
    if ((rc != 0)); then
        printf 'Test failed; private scratch retained: %s\n' "$scratch" >&2
    else
        rm -rf -- "$scratch"
    fi
    exit "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir -m 700 "$scratch/socket"
# Keep the local Unix socket path within common Linux sockaddr_un limits.
(( ${#scratch} + 24 < 104 )) || die "Test scratch path is too long."
# Ignore existing PostgreSQL client/service settings; explicitly use the new test role and socket.
unset PGHOST PGHOSTADDR PGPORT PGDATABASE PGUSER PGPASSWORD PGPASSFILE PGSERVICE PGSERVICEFILE PGOPTIONS PGSSLMODE PGSSLROOTCERT PGSSLCERT PGSSLKEY PGCONNECT_TIMEOUT PGAPPNAME PGTARGETSESSIONATTRS
"$pgbin/initdb" -D "$scratch/db" -U zalgps_test --auth-local=trust --auth-host=reject --no-locale --encoding=UTF8 >"$scratch/init.log" 2>&1
"$pgbin/pg_ctl" -D "$scratch/db" -l "$scratch/server.log" -o "-c listen_addresses='' -c unix_socket_directories=$scratch/socket -c unix_socket_permissions=0700 -p 5432 -c shared_buffers=64MB -c max_connections=20 -c work_mem=4MB" -w -t 30 start
export ZALGPS_TEST_DATABASE_URL="host=$scratch/socket port=5432 dbname=postgres user=zalgps_test sslmode=disable"
export GOMAXPROCS=2 GOMEMLIMIT=1536MiB
cd "$root"
test -z "$(gofmt -l .)" || die "Go formatting check failed."
timeout --kill-after=10s 120s go mod verify
timeout --kill-after=10s 300s go test -p 2 -parallel 2 -race -count=1 -v ./...
timeout --kill-after=10s 120s go vet -p 2 ./...
timeout --kill-after=10s 120s go build -p 2 -o "$scratch/zalgpsd" ./cmd/zalgpsd
printf '%s\n' "Isolated PostgreSQL integration checks passed."
