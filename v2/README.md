# ZalGPS V2 — durable Go pilot

Experimental, loopback-only. Legacy Java remains untouched. See [architecture](docs/architecture.md) and [security policy](SECURITY.md).

Native JSON ingest stores fixes in PostgreSQL with device/tenant scope, hashed credentials, transaction commit before HTTP 202, idempotent duplicates (HTTP 200) and late fixes preserved without regressing latest fix time. Not Traccar/OsmAnd or hardware protocol compatible yet. Health is liveness only.

Use a disposable dedicated PostgreSQL database. Set the separate migrator, provisioner and runtime DSNs privately according to [DB-02 role bootstrap](internal/storage/DB-02_BOOTSTRAP.md), plus ZALGPS_TENANT_ID, ZALGPS_DEVICE_ID and ZALGPS_DEVICE_TOKEN. Generate each device token using a CSPRNG (32 bytes, lowercase hex). Do not expose tokens or real customer data.

```sh
go build -o bin/zalgpsd ./cmd/zalgpsd
./bin/zalgpsd migrate
./bin/zalgpsd enroll
./bin/zalgpsd serve
```

POST /v2/positions with Bearer token and application/json: fixTime RFC3339, latitude/longitude WGS84 and optional speedKph. GET /v2/positions/latest reads only the authenticated device. Use rotate or revoke with private environment variables; no HTTP admin enrollment endpoint.

Run go test -race ./... and go vet ./... . Set ZALGPS_TEST_DATABASE_URL only to a disposable database to run integration tests. Durability assumes normal PostgreSQL fsync and durable storage. Versioned migrations are implemented; see [database migrations](docs/migrations.md) for upgrade and checksum rules. Integration migration tests require a disposable PostgreSQL cluster and CREATEDB privilege. PostGIS, partitioning and user management remain future gates. Role-separated PostgreSQL integration tests require the disposable bootstrap described above. Use a supported patched Go toolchain.

For an automated isolated PostgreSQL integration run, see [local test profile](docs/local-test.md).

## Pre-commit secret guard

Run the staged secret scan before committing:

```sh
./scripts/check-staged.sh
```

This runs `gitleaks git --staged --redact=100` (gitleaks 8.30.1). Behavior is fail-closed:
missing tool, scan error, or any finding -> non-zero exit. No secret values or finding details are printed.
Review findings privately; the script does not scan history. It is a manual gate, not an installed hook.

## Agent coordination

Start with [AGENTS.md](AGENTS.md), [master roadmap](ROADMAP.md), [execution TODO](TODO.md), [deployment plan](docs/deployment.md), [acceptance gates](docs/acceptance.md), [protocol matrix](docs/protocol-matrix.md) and [handoff](docs/handoff.md), [threat model](docs/security-threat-model.md). Public repository: do not commit private deployment details or real device data. Production is NOT READY.
