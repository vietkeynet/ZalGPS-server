# DB-02 role bootstrap

Before migration, DBA must create cluster-wide `zalgps_functions` as NOLOGIN and grant membership to `zalgps_migrator` with authority to transfer function ownership. DBA must also create `zalgps_migrator` as LOGIN and configure migrator DSN for it, plus `zalgps_provisioner` and `zalgps_runtime` as LOGIN roles. Keep credentials outside this repository. For a fresh DB, `0001` runs as `zalgps_migrator`, which therefore owns schema `zalgps_v2` and tables it creates. For pre-ledger pilot adoption, verify the migrator owns the existing schema and all existing tables, or arrange DBA-approved ownership transfer before migration; structural adoption validation does not change object owners.

## Role responsibilities

| Role | Login | Access |
| --- | --- | --- |
| `zalgps_migrator` | Yes | Owns `zalgps_v2` schema and tables; applies migrations and performs schema administration. |
| `zalgps_functions` | No | Cluster-wide role; owns narrowly scoped SECURITY DEFINER functions; not a table owner. Provisioned once by DBA. |
| `zalgps_provisioner` | Yes | EXECUTE on device enrollment, rotation, and revocation functions only. |
| `zalgps_runtime` | Yes | EXECUTE on ingestion and latest-position functions only. |

## DBA setup

DBA creates `zalgps_functions` once at cluster scope as NOLOGIN, and grants membership to `zalgps_migrator` with required authority for function ownership transfers. Create the three LOGIN roles before applying migrations. Keep passwords private; do not place real secrets in SQL files or shell history. In a dedicated database cluster, an authorized DBA can bootstrap roles with the following SQL (do not run blindly on an existing cluster):

```sql
CREATE ROLE zalgps_functions NOLOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS NOREPLICATION;
CREATE ROLE zalgps_migrator LOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS NOREPLICATION;
CREATE ROLE zalgps_provisioner LOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS NOREPLICATION;
CREATE ROLE zalgps_runtime LOGIN NOSUPERUSER NOCREATEROLE NOCREATEDB NOBYPASSRLS NOREPLICATION;
GRANT zalgps_functions TO zalgps_migrator;
```

Assign the dedicated database to the migrator and grant CONNECT to application roles. In an interactive `psql` session (not in a SQL migration), run each command separately; `\password` prompts for the password:

```text
\password zalgps_migrator
\password zalgps_provisioner
\password zalgps_runtime
```

`zalgps_migrator` must own schema `zalgps_v2` and its tables and have required schema/table DDL privileges. It does not need CREATEROLE: DBA pre-provisions `zalgps_functions` cluster-wide and grants membership. Restrict CREATEROLE and ADMIN OPTION to authorized administrators; do not grant them to runtime/provisioner. Review the full role membership graph and ensure runtime/provisioner are not members of `zalgps_functions`, `zalgps_migrator`, or any role that grants equivalent access. Ensure no other roles have CREATE on `zalgps_v2`, and no untrusted role can write to it; SECURITY DEFINER functions use it in `search_path`. Migration revokes PUBLIC and application CREATE and checks these application-role memberships, but does not validate every indirect role or ACL path.

For a fresh database, run 0001 as `zalgps_migrator`; schema and tables then belong to it. Pre-ledger pilot adoption only validates structure; it does not transfer ownership. Before adopting, ensure `zalgps_migrator` owns existing schema and tables, or have DBA perform approved ownership transfer. 0002 requires pre-existing `zalgps_provisioner` and `zalgps_runtime` LOGIN roles and verifies `zalgps_functions` exists as NOLOGIN/NOBYPASSRLS and `zalgps_migrator` can assume it.

Apply 0002 through the normal migrator DSN. Preflight requires the cluster-wide function role to be NOLOGIN/NOBYPASSRLS and verifies migrator membership before granting its narrow table access and installing FORCE RLS policies. It temporarily grants CREATE on `zalgps_v2` to `zalgps_functions` for function ownership transfers, then revokes CREATE after the final transfer; the role retains USAGE for function execution. It revokes PUBLIC and application schema CREATE and all direct table privileges from PUBLIC/runtime/provisioner before narrow grants. It checks membership of runtime/provisioner in the named migrator/function roles, not the entire role graph or every possible effective ACL. DBA must review these broader membership and ACL paths; no untrusted role may write the schema or assume the function owner role. Migration creates no roles or passwords and requires no BYPASSRLS.

## Verification

Run as an administrator after migration:

```sql
SELECT rolname, rolcanlogin, rolbypassrls
FROM pg_catalog.pg_roles
WHERE rolname IN ('zalgps_migrator','zalgps_functions','zalgps_provisioner','zalgps_runtime')
ORDER BY rolname;

SELECT role_name, privilege,
       pg_catalog.has_table_privilege(role_name, 'zalgps_v2.devices', privilege) AS devices,
       pg_catalog.has_table_privilege(role_name, 'zalgps_v2.positions', privilege) AS positions
FROM (VALUES ('zalgps_runtime'), ('zalgps_provisioner')) AS roles(role_name)
CROSS JOIN (VALUES ('SELECT'), ('INSERT'), ('UPDATE'), ('DELETE')) AS privileges(privilege);

SELECT a.grantee, a.privilege_type
FROM pg_catalog.pg_namespace n
CROSS JOIN LATERAL pg_catalog.aclexplode(n.nspacl) a
WHERE n.nspname = 'zalgps_v2' AND a.privilege_type = 'CREATE'
  AND a.grantee = 0;
```

No rows means PUBLIC has no schema CREATE grant. Also review schema ACL rows for other grantees and the full role membership graph as an administrator; do not infer those checks from this query or the migration preflight.

```sql
SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='zalgps_v2' AND c.relname IN ('tenants','devices','positions')
ORDER BY c.relname;

SELECT p.proname,
       pg_catalog.has_function_privilege('zalgps_runtime', p.oid, 'EXECUTE') AS runtime_exec,
       pg_catalog.has_function_privilege('zalgps_provisioner', p.oid, 'EXECUTE') AS provisioner_exec
FROM pg_catalog.pg_proc p
JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace
WHERE n.nspname='zalgps_v2'
ORDER BY p.proname;
```


Expected: runtime/provisioner have no protected tenant/device/position table privileges; each can execute only its assigned functions. All three protected tables have RLS enabled and forced; all four roles have `rolbypassrls = false`.

Runtime has read-only access to the migration ledger for exact checksum startup verification. The test-only CREATEDB privilege and test group belong only in disposable clusters, never production.

Provisioner is a trusted cluster-wide device administration identity, not a tenant-scoped user. Application identities have no direct protected table grants; the NOLOGIN function owner has narrowly granted table operations and the functions derive tenant and device from the authenticated token. This is not a general-purpose tenant SQL session policy. Reassess access when adding user APIs.
