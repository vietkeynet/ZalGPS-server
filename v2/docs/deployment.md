# Deployment and environment plan (public-safe)

This is a design and operational gate, not a live deployment manifest. No private network addresses, hosts, credentials or paths are stored here. Maintain an authorized private inventory and release configuration separately.

| Environment | Purpose | Boundary | Status |
|---|---|---|---|
| DEV | local binary, synthetic fixtures | loopback, dedicated disposable DB, isolated checkout | pilot verified |
| CI | compile/test/race/vet, isolated PostgreSQL | GitHub-hosted only, read-only token, no deployment secrets | baseline PASS |
| STAGING | integration, hardware lab, load and recovery | dedicated DB/storage/credentials, restricted network, separate ports | planned |
| PROD | customer workloads | dedicated service/DB/backup, TLS, explicit approval, rollback | NOT READY |
| ON-PREM | customer-controlled edition | installer/Compose/native, customer secrets, offline upgrades where needed | planned |

## Deployment topology
Core initially one Go binary; optionally split into gateway/API/worker when measured. PostgreSQL on supported durable block storage; never assume shared file storage is suitable for PGDATA. PostGIS extension requires explicit availability check. Use a dedicated database and separate migration/provisioner/runtime roles; do not reuse other projects' schema, credentials or data. Redis, object storage and message bus optional later. No GPU. Web/API behind TLS reverse proxy; device TCP/UDP exposed only for verified protocols and required ports. HTTP reverse proxies do not automatically forward arbitrary GPS TCP/UDP. Use L4 forwarding/firewall or dedicated gateway. Legacy and V2 ports must not conflict; maintain private port registry.

## Provisioning checklist
- Preflight CPU/RAM/disk/IO, OS/arch, supported toolchains, clock sync, port occupancy, database version, extension support, restore capacity. Set project resource budgets, monitor host headroom; stop if another service is at risk.
- Source/worktrees on dedicated development storage; test data and secrets in private isolated paths. No installation or global upgrade without review.
- Build a pinned artifact from exact CI-green SHA. No untrusted PR code on internal runners; private deployment pipeline receives reviewed immutable artifacts only. Keep deploy credentials out of public CI. Add non-root execution, restricted filesystem, limits, readiness and health; current /healthz is liveness only.
- Use environment variables or secret files from a private manager, random per-device credentials, TLS/verify-full for remote DB, separate environment identities. Disable default accounts, enforce egress/ingress allowlists, rotate secrets. No public database, no real data in staging without authorization.
- Migrate with backup and tested upgrade path; use forward-compatible schema changes. Verify API health, ingest, DB persistence and scope; watch error rates, queue lag, disk/WAL and reconnects. Stop on scope leak, durable data loss, failing migration, exhausted storage or latency breach.
- Backup database with tested logical/physical strategy, encrypt and keep off-host copy, define retention and deletion, verify recovery in separate DB; recovery targets must be measured. Do not claim HA based only on a replica.
- Rollback: stop routing new devices to V2, retain durable accepted records and reconcile; revert binary/config only if schema compatible. No down migration or delete of customer data without reviewed recovery plan. Preserve legacy endpoint until migration gates pass.

## Proposed test resource envelopes (not measured capacity)
DEV/CI: 2 vCPU/4 GiB budget for pilot test; staging benchmark: start at 2–4 vCPU/4–8 GiB with dedicated DB and record exact resources. Pilot target 1,000 devices / 15 s ~67 records/s. 10,000 / 15 s ~667/s; 100,000 ~6,667/s. At 500 logical bytes/fix, 10,000 generates 28.8 GB/day before indexes/WAL/replicas/backups. Size DB independently and enforce retention; verify actual disk usage, no unbounded stress tests on shared hosts. Video is separately sized.

## Release gates
See TODO REL-01 and acceptance.md. DEV can proceed with synthetic data; STAGING restricted only after scoped prerequisites; no production traffic or public listener before security, restore, compatibility, load, legal and approval gates. Private infrastructure inventory must identify owner, environment, artifact SHA, DB/storage, ports, TLS, secrets reference, backup schedule, monitoring and rollback (no secret values).
