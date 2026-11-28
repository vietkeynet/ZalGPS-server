# DB-03 PostGIS bootstrap

PostGIS extension is provisioned by DBA per database, not by migration. Install it in `template1` for disposable test databases and in every existing target database before migration:

```sql
CREATE EXTENSION postgis WITH SCHEMA public;
```

The local smoke-tested pilot image uses PostgreSQL 16.9 and PostGIS 3.5.2; full integration and security acceptance are separate. Image: `postgis/postgis:16-3.5`, digest `sha256:94146ac37bc61e2322f88016056c5920729cb8c64c8542ed590af8fc2abdac07`. Migration parses three numeric `extversion` components, permits optional `+` build metadata, rejects prerelease suffixes, and accepts stable versions `>= 3.5.2` and `< 4.0.0`; malformed or out-of-range versions fail closed. It requires extension schema `public` and denies CREATE there to PUBLIC, `zalgps_runtime`, and `zalgps_provisioner`.

Run upgrade only after backup and restore verification. Migration adds a stored generated geography column and GiST index; existing positions are backfilled in a table rewrite, with WAL and lock impact. Pilot assumes bounded table size. Extension remains DBA-owned; generated column and index are table/migrator-owned.

After version 3 is applied, do not start a binary embedding only migrations 0001/0002: it rejects the newer ledger. Recover by restoring backup or using a reviewed compatible binary.
