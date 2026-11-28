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
command -v psql >/dev/null || die "Missing prerequisite: psql"
pgbin=$(pg_config --bindir)
for tool in initdb pg_ctl postgres; do
    [[ -x "$pgbin/$tool" ]] || die "Missing PostgreSQL tool: $tool"
done
[[ $("$pgbin/postgres" --version) =~ PostgreSQL\)\ 16\. ]] || die "PostgreSQL 16 is required for this test profile."
postgis_control=$(pg_config --sharedir)/extension/postgis.control
[[ -r $postgis_control ]] || die "PostGIS extension files are missing ($postgis_control); install PostgreSQL 16 PostGIS packages locally before running isolated tests."
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
# Ignore existing PostgreSQL client/service settings; bootstrap only through private Unix socket.
unset PGHOST PGHOSTADDR PGPORT PGDATABASE PGUSER PGPASSWORD PGPASSFILE PGSERVICE PGSERVICEFILE PGOPTIONS PGSSLMODE PGSSLROOTCERT PGSSLCERT PGSSLKEY PGCONNECT_TIMEOUT PGAPPNAME PGTARGETSESSIONATTRS
"$pgbin/initdb" -D "$scratch/db" -U zalgps_bootstrap --auth-local=trust --auth-host=reject --no-locale --encoding=UTF8 >"$scratch/init.log" 2>&1
"$pgbin/pg_ctl" -D "$scratch/db" -l "$scratch/server.log" -o "-c listen_addresses='' -c unix_socket_directories=$scratch/socket -c unix_socket_permissions=0700 -p 5432 -c shared_buffers=64MB -c max_connections=20 -c work_mem=4MB" -w -t 30 start
"$pgbin/psql" -X -v ON_ERROR_STOP=1 -h "$scratch/socket" -p 5432 -U zalgps_bootstrap -d template1 -c 'CREATE EXTENSION postgis;'
version=$("$pgbin/psql" -X -At -v ON_ERROR_STOP=1 -h "$scratch/socket" -p 5432 -U zalgps_bootstrap -d template1 -c "SELECT extversion FROM pg_extension WHERE extname = 'postgis' AND extversion ~ '^[0-9]+\\.[0-9]+\\.[0-9]+(\\+[A-Za-z0-9.-]+)?$' AND string_to_array(split_part(extversion, '+', 1), '.')::int[] >= ARRAY[3,5,2] AND string_to_array(split_part(extversion, '+', 1), '.')::int[] < ARRAY[4,0,0];")
[[ -n $version ]] || die "PostGIS version must be >= 3.5.2 and < 4.0.0."
"$pgbin/psql" -X -v ON_ERROR_STOP=1 -h "$scratch/socket" -p 5432 -U zalgps_bootstrap -d postgres <<'SQL'
CREATE ROLE zalgps_functions NOLOGIN;
CREATE ROLE zalgps_migrator LOGIN CREATEDB;
CREATE ROLE zalgps_provisioner LOGIN;
CREATE ROLE zalgps_runtime LOGIN;
GRANT zalgps_functions TO zalgps_migrator;
CREATE ROLE zalgps_test_schema_create NOLOGIN;
GRANT zalgps_test_schema_create TO zalgps_runtime;
CREATE DATABASE zalgps_test OWNER zalgps_migrator;
SQL
version=$("$pgbin/psql" -X -At -v ON_ERROR_STOP=1 -h "$scratch/socket" -p 5432 -U zalgps_bootstrap -d zalgps_test -c "SELECT extversion FROM pg_extension WHERE extname = 'postgis' AND extversion ~ '^[0-9]+\\.[0-9]+\\.[0-9]+(\\+[A-Za-z0-9.-]+)?$' AND string_to_array(split_part(extversion, '+', 1), '.')::int[] >= ARRAY[3,5,2] AND string_to_array(split_part(extversion, '+', 1), '.')::int[] < ARRAY[4,0,0];")
[[ -n $version ]] || die "Inherited PostGIS version must be >= 3.5.2 and < 4.0.0."
"$pgbin/psql" -X -v ON_ERROR_STOP=1 -h "$scratch/socket" -p 5432 -U zalgps_bootstrap -d postgres -c 'GRANT CONNECT ON DATABASE zalgps_test TO zalgps_runtime, zalgps_provisioner;'
conn="host=$scratch/socket port=5432 dbname=zalgps_test sslmode=disable"
export ZALGPS_MIGRATOR_DATABASE_URL="$conn user=zalgps_migrator"
export ZALGPS_PROVISIONER_DATABASE_URL="$conn user=zalgps_provisioner"
export ZALGPS_RUNTIME_DATABASE_URL="$conn user=zalgps_runtime"
export ZALGPS_TEST_DATABASE_URL="$conn user=zalgps_migrator"
export ZALGPS_REQUIRE_ROLE_TESTS=1
export GOMAXPROCS=2 GOMEMLIMIT=1536MiB
cd "$root"
test -z "$(gofmt -l .)" || die "Go formatting check failed."
timeout --kill-after=10s 120s go mod verify
timeout --kill-after=10s 300s go test -p 2 -parallel 2 -race -count=1 -v ./...
timeout --kill-after=10s 120s go vet -p 2 ./...
timeout --kill-after=10s 120s go build -p 2 -o "$scratch/zalgpsd" ./cmd/zalgpsd
printf '%s\n' "Isolated PostgreSQL integration checks passed."
