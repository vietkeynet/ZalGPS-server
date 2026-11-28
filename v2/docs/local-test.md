# Isolated development and integration test profile

Use a supported patched Go 1.26 toolchain, PostgreSQL 16 server binaries (including pg_config, initdb and pg_ctl), PostGIS >= 3.5.2 and < 4.0.0 extension files for PostgreSQL 16, Bash and standard Linux utilities. No Docker, shared database, root account, existing database connection string, deployment credentials or real device data are required. Run only trusted, reviewed source locally; public/untrusted pull requests belong on hosted CI.

From the V2 module root:

```sh
bash scripts/test-isolated.sh
```

The script always initializes a fresh PostgreSQL cluster with a dedicated synthetic test role. It ignores existing PostgreSQL connection settings, binds **no TCP listener**, uses a private Unix socket and overrides the test DSN. The integration suite creates only disposable databases in that cluster. The script shuts down the cluster and removes only its newly created temporary directory after success; failures retain private diagnostic files for local inspection. Do not publish logs or retained data. If shutdown fails, inspect and stop only the identified temporary cluster before removing its scratch directory.

Default scratch is /tmp; optionally choose an existing writable local scratch directory via ZALGPS_TEST_TMPDIR. Use a short path on suitable local storage, not shared NFS for PGDATA. Minimum free scratch 2 GiB; verify additional capacity for Go module/build caches and test artifacts. The test recipe limits Go concurrency, sets a soft Go memory target, configures small PostgreSQL buffers and uses timeouts. These are **not** hard CPU/RAM quotas; use an isolated cgroup/VM with an approved resource budget if strict quotas are needed. Do not stress-test a shared development host. No system-wide installation, production service, firewall, or port changes.

The script runs format, module verification, race-enabled unit and PostgreSQL integration tests, vet and build. Confirm that TestVersionedMigrations and TestPostgresIntegration ran without SKIP. This is the PostgreSQL 16 development recipe, not a deployment or production recovery test. PostGIS is now required; the script fails before starting a cluster if extension files are absent, and provisions it only in the disposable cluster. Staging, backup/restore and hardware tests remain separate acceptance gates.

For strict whole-process-tree CPU/RAM quotas, see [resource profiles](resource-profile.md) and the fail-closed wrapper. Hard quota runtime acceptance requires a suitable delegated cgroup host; the regular test recipe is not a substitute.
