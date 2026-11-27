# Resource profile — development, CI and staging

These are proposed budgets, not measured production capacity. Keep project source and build caches on the selected development volume. Never assume network file storage is suitable for PostgreSQL PGDATA.

| Profile | Proposed budget | Isolation and evidence |
|---|---|---|
| DEV | 2 vCPU, 4 GiB RAM, 2 GiB free scratch minimum | Disposable PostgreSQL 16, private Unix socket only. Soft-limit recipe verified. |
| CI | Hosted runner allocation | GitHub-hosted, read-only token, disposable PostgreSQL service. Integration/race CI verified; no hard quota claim. |
| STAGING | Start at 2–4 vCPU, 4–8 GiB, separate durable DB storage | Dedicated service identity, hard resource budget, restricted ingress, TLS, readiness, backup and restore drill. NOT VERIFIED. |
| PROD | Size from measured load and recovery | NOT READY; release gates apply. |

## Development limits

The local test recipe sets GOMAXPROCS=2, GOMEMLIMIT=1536MiB, PostgreSQL shared_buffers=64MB, max_connections=20 and work_mem=4MB, and command timeouts. These are not hard limits on the whole process tree. Use an approved isolated VM or delegated cgroup for strict quotas. Do not stress-test a shared host.

For a strict local test, use the wrapper from the V2 module:

```sh
bash scripts/test-with-quota.sh
```

The wrapper requires Linux cgroup v2 and an available systemd user manager with delegated CPU and memory controllers. It creates a transient user scope capped at two CPUs and 4 GiB RAM, then checks the actual scope's cpu.max and memory.max before executing the disposable test recipe. If unavailable or limits cannot be read or are weaker than required, it exits nonzero without starting the workload. No fallback to unrestricted execution. Do not use ulimit as a whole-process-tree quota. This is not a security sandbox. Strict runtime success path remains NOT VERIFIED on a suitable host.

Use ZALGPS_TEST_TMPDIR to select an existing short writable local scratch directory, optionally a suitably sized RAM filesystem for disposable tests. Do not create a persistent cluster on shared NFS. Test scratch is cleaned by the test script after success; on failure inspect only the private retained directory. RAM storage is unsuitable for production PGDATA.

## Staging gate

Use a dedicated DB and storage with explicit quotas and capacity monitoring, separate runtime/migration credentials, non-root process, restricted network and TLS. Verify actual CPU/RAM and disk limits, recoverability, port registry, health/readiness, observability and rollback before deployment. No staging or production environment is provisioned by these recipes.
