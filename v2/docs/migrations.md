# Database migrations (DB-01)

The Go pilot uses immutable SQL files under internal/storage/migrations/ embedded into the binary. Name the next file 0002_description.sql, then 0003, etc. Never alter a previously applied migration. Every entry stores version, filename, SHA-256 checksum and applied time. Migrations execute inside a READ COMMITTED transaction, serialized by a transaction-scoped advisory lock. Only a dedicated migrator should execute them. Do not perform concurrent manual DDL.

Fresh databases are initialized by migration 0001. Existing pre-ledger pilot schemas are adopted only if their catalog structure matches the immutable 0001 reference (relations, columns, defaults, constraints, indexes, trigger/policy presence). Validation constructs a reference schema in a savepoint and rolls it back; the migrator needs permission to create that schema. Mismatch, missing relations, noncontiguous/unknown versions, empty existing ledger or changed checksum fail closed. No automatic repair or destructive downgrade. Migration 0002 establishes DB-02 ownership, grants and RLS; see role bootstrap below. No ongoing schema-drift monitoring is performed. At startup the server verifies the complete ledger and existence of required core relations, not a full live-schema comparison.

Before an upgrade: back up, verify restore on a separate database, stop unmanaged DDL, review the exact migration and application compatibility, and run migrate using the authorized migrator. Verify startup and acceptance tests. If a migration fails the transaction rolls back; after a successful migration use only a schema-compatible binary or the reviewed recovery plan. Never edit the ledger manually to bypass a checksum.

## Role bootstrap (DB-02)

Migration 0002 requires the cluster-wide `zalgps_functions` role, provisioned once by an authorized DBA as NOLOGIN with NOBYPASSRLS and membership granted to `zalgps_migrator` with authority to transfer function ownership. It defines SECURITY DEFINER functions for data access, enables and forces row-level security on `tenants`, `devices` and `positions`, and grants schema USAGE plus function EXECUTE to application login roles.

Prerequisites (enforced by DBA before migration):
* `zalgps_functions` exists at cluster scope with `NOLOGIN` and `NOBYPASSRLS`. DBA grants membership to `zalgps_migrator`; migration preflight verifies membership. Do not create this role per database or inside 0002.
* `zalgps_migrator` LOGIN role exists, owns schema `zalgps_v2` and all its tables, and has DDL privileges. CREATEROLE is not required; role creation and membership are DBA bootstrap operations.
* `zalgps_provisioner` and `zalgps_runtime` LOGIN roles exist with private passwords (never in repo); 0002 grants them schema usage and function EXECUTE.
* Schema `zalgps_v2` must have no PUBLIC CREATE privilege and must not be writable by runtime/provisioner or other untrusted roles, because SECURITY DEFINER functions use it in `search_path`.
* For fresh DBs: run 0001 as `zalgps_migrator` to establish ownership. For pre-ledger pilot adoption: structural validation does not change ownership; ensure `zalgps_migrator` owns existing schema/tables or have DBA transfer ownership before migration.

See `v2/internal/storage/DB-02_BOOTSTRAP.md` for the complete bootstrap recipe and verification queries.
Integration tests create randomly named disposable databases on the configured test cluster (requires CREATEDB); they drop only the databases they create. Never point ZALGPS_TEST_DATABASE_URL at production or shared customer infrastructure. Tests cover fresh/repeated/concurrent migration, pre-ledger adoption preserving a synthetic row, drift rejection with no ledger left behind, changed checksum, missing core relation, empty existing ledger and native ingest. PostgreSQL 16 is the verified test target. Future nontransactional DDL needs a separately reviewed runner design.

## PostGIS (DB-03)

PostGIS is DBA-provisioned per database in schema `public`; migrations never install or own the extension. The DBA bootstrap must install PostGIS in `template1` so disposable databases created by tests inherit it, and in each existing target database before upgrade (for example, `CREATE EXTENSION postgis WITH SCHEMA public`). Migration 0003 fails closed unless extension exists in `public`, its `extversion` parses as three numeric components with optional `+` build metadata only (prerelease suffixes rejected), and stable numeric version is `>= 3.5.2` and `< 4.0.0`; `public` must deny CREATE to PUBLIC, `zalgps_runtime`, and `zalgps_provisioner`. Test candidate: PostgreSQL 16.9 / PostGIS 3.5.2; local smoke-tested image `postgis/postgis:16-3.5`, digest `sha256:94146ac37bc61e2322f88016056c5920729cb8c64c8542ed590af8fc2abdac07`.

Tests must create the extension in `template1` before creating randomized databases. Migration remains transactional; do not add concurrent or nontransactional index DDL.

Migration 0003 adds a stored geography point from existing longitude/latitude and a GiST index. This backfills the full positions table and can cause a table rewrite, WAL growth, and locks; pilot assumes bounded table size. Back up and verify restore before applying. Do not roll back to a binary that embeds only migrations 0001/0002 after 0003 is applied: it rejects the newer ledger. Recover by restoring the backup or using a reviewed compatible binary.
