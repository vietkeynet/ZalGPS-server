package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vietkeynet/ZalGPS-server/v2/internal/ingest"
)

func randomKey(t *testing.T) [32]byte {
	t.Helper()
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		t.Fatal(e)
	}
	k, e := ingest.HashToken(hex.EncodeToString(b[:]))
	if e != nil {
		t.Fatal(e)
	}
	return k
}
func TestPostgresIntegration(t *testing.T) {
	migratorDSN := os.Getenv("ZALGPS_MIGRATOR_DATABASE_URL")
	runtimeDSN := os.Getenv("ZALGPS_RUNTIME_DATABASE_URL")
	provisionerDSN := os.Getenv("ZALGPS_PROVISIONER_DATABASE_URL")
	if migratorDSN == "" || runtimeDSN == "" || provisionerDSN == "" {
		if os.Getenv("ZALGPS_REQUIRE_ROLE_TESTS") == "1" {
			t.Fatal("required role-separated integration database not configured")
		}
		t.Skip("disposable role-separated integration database not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrator, e := Open(ctx, migratorDSN)
	if e != nil {
		t.Fatal(e)
	}
	if e = migrator.Migrate(ctx); e != nil {
		migrator.Close()
		t.Fatal(e)
	}
	migrator.Close()
	runtime, e := Open(ctx, runtimeDSN)
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	provisioner, e := Open(ctx, provisionerDSN)
	if e != nil {
		t.Fatal(e)
	}
	defer provisioner.Close()
	if e = runtime.CheckRuntimeSchema(ctx); e != nil {
		t.Fatal(e)
	}
	var suffix [8]byte
	if _, e = rand.Read(suffix[:]); e != nil {
		t.Fatal(e)
	}
	a, b := "test-a-"+hex.EncodeToString(suffix[:]), "test-b-"+hex.EncodeToString(suffix[:])
	ka, kb, kc := randomKey(t), randomKey(t), randomKey(t)
	for _, x := range []struct {
		tenant string
		k      [32]byte
	}{{a, ka}, {b, kb}} {
		if e = provisioner.Enroll(ctx, x.tenant, "same-device", x.k); e != nil {
			t.Fatal(e)
		}
	}
	if e = provisioner.Enroll(ctx, b, "other-device", ka); e == nil {
		t.Fatal("duplicate token allowed")
	}
	lat, lon := 0.0, 0.0
	now := time.Now().UTC().Truncate(time.Microsecond)
	pos := ingest.Position{FixTime: now, Latitude: &lat, Longitude: &lon}
	if ok, e := runtime.Accept(ctx, ka, pos, now); e != nil || !ok {
		t.Fatalf("insert: %v %v", ok, e)
	}
	if ok, e := runtime.Accept(ctx, ka, pos, now); e != nil || ok {
		t.Fatalf("duplicate: %v %v", ok, e)
	}
	late := pos
	late.FixTime = now.Add(-time.Hour)
	if ok, e := runtime.Accept(ctx, ka, late, now); e != nil || !ok {
		t.Fatalf("late: %v %v", ok, e)
	}
	if _, e = runtime.Latest(ctx, kb); !errors.Is(e, ingest.ErrNotFound) {
		t.Fatalf("cross tenant read: %v", e)
	}
	if ok, e := runtime.Accept(ctx, kb, pos, now); e != nil || !ok {
		t.Fatalf("tenant scoped dedup: %v %v", ok, e)
	}
	r, e := runtime.Latest(ctx, ka)
	if e != nil || r.TenantID != a || !r.FixTime.Equal(now) {
		t.Fatalf("latest: %v", e)
	}
	if e = provisioner.Rotate(ctx, a, "same-device", kc); e != nil {
		t.Fatal(e)
	}
	if _, e = runtime.Latest(ctx, ka); !errors.Is(e, ingest.ErrUnauthorized) {
		t.Fatal("old token still active")
	}
	if _, e = runtime.Latest(ctx, kc); e != nil {
		t.Fatal(e)
	}
	if e = provisioner.Revoke(ctx, a, "same-device"); e != nil {
		t.Fatal(e)
	}
	if _, e = runtime.Accept(ctx, kc, pos, now); !errors.Is(e, ingest.ErrUnauthorized) {
		t.Fatal("revoked token accepted")
	}
	if _, e = runtime.Latest(ctx, kb); e != nil {
		t.Fatal("other tenant affected")
	}
}

func TestPostgresRoleBoundaries(t *testing.T) {
	dsn := map[string]string{
		"runtime":     os.Getenv("ZALGPS_RUNTIME_DATABASE_URL"),
		"provisioner": os.Getenv("ZALGPS_PROVISIONER_DATABASE_URL"),
	}
	if dsn["runtime"] == "" || dsn["provisioner"] == "" {
		if os.Getenv("ZALGPS_REQUIRE_ROLE_TESTS") == "1" {
			t.Fatal("required role-separated integration database not configured")
		}
		t.Skip("disposable role-separated integration database not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for role, url := range dsn {
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		checks := []string{"SELECT * FROM zalgps_v2.devices", "SELECT token_hash FROM zalgps_v2.devices", "INSERT INTO zalgps_v2.devices(tenant_id,device_id,token_hash) VALUES ('x','y',decode('00','hex'))", "UPDATE zalgps_v2.devices SET revoked=true", "CREATE TABLE zalgps_v2.forbidden(id int)"}
		if role == "runtime" {
			checks = append(checks, "SELECT * FROM zalgps_v2.positions", "SELECT zalgps_v2.enroll_device('x','y',decode('00','hex'))")
		} else {
			checks = append(checks, "SELECT zalgps_v2.accept_position(decode('00','hex'),now(),now(),decode('00','hex'),0,0,NULL)", "SELECT zalgps_v2.latest_position(decode('00','hex'))")
		}
		for _, query := range checks {
			if _, err = pool.Exec(ctx, query); err == nil {
				pool.Close()
				t.Fatalf("%s unexpectedly allowed unauthorized SQL", role)
			} else {
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
					pool.Close()
					t.Fatalf("%s unauthorized SQL returned unexpected error class", role)
				}
			}
		}
		pool.Close()
	}
}

func TestPostgresDirectFunctionDeny(t *testing.T) {
	dsn := map[string]string{
		"runtime":     os.Getenv("ZALGPS_RUNTIME_DATABASE_URL"),
		"provisioner": os.Getenv("ZALGPS_PROVISIONER_DATABASE_URL"),
	}
	if dsn["runtime"] == "" || dsn["provisioner"] == "" {
		if os.Getenv("ZALGPS_REQUIRE_ROLE_TESTS") == "1" {
			t.Fatal("required role-separated integration database not configured")
		}
		t.Skip("disposable role-separated integration database not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for role, url := range dsn {
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		// authenticate_device must be denied to both runtime and provisioner
		if _, err = pool.Exec(ctx, "SELECT zalgps_v2.authenticate_device(decode('00','hex'))"); err == nil {
			pool.Close()
			t.Fatalf("%s unexpectedly allowed authenticate_device", role)
		} else {
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
				pool.Close()
				t.Fatalf("%s authenticate_device returned unexpected error class", role)
			}
		}
		pool.Close()
	}
}

func TestPostgresTenantIsolation(t *testing.T) {
	migratorDSN := os.Getenv("ZALGPS_MIGRATOR_DATABASE_URL")
	runtimeDSN := os.Getenv("ZALGPS_RUNTIME_DATABASE_URL")
	provisionerDSN := os.Getenv("ZALGPS_PROVISIONER_DATABASE_URL")
	if migratorDSN == "" || runtimeDSN == "" || provisionerDSN == "" {
		if os.Getenv("ZALGPS_REQUIRE_ROLE_TESTS") == "1" {
			t.Fatal("required role-separated integration database not configured")
		}
		t.Skip("disposable role-separated integration database not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrator, e := Open(ctx, migratorDSN)
	if e != nil {
		t.Fatal(e)
	}
	if e = migrator.Migrate(ctx); e != nil {
		migrator.Close()
		t.Fatal(e)
	}
	migrator.Close()
	runtime, e := Open(ctx, runtimeDSN)
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	provisioner, e := Open(ctx, provisionerDSN)
	if e != nil {
		t.Fatal(e)
	}
	defer provisioner.Close()
	// Provision two tenants with same device ID
	var suffix [8]byte
	if _, e = rand.Read(suffix[:]); e != nil {
		t.Fatal(e)
	}
	tenantA := "tenant-a-" + hex.EncodeToString(suffix[:])
	tenantB := "tenant-b-" + hex.EncodeToString(suffix[:])
	deviceID := "same-device"
	keyA, keyB := randomKey(t), randomKey(t)
	if e = provisioner.Enroll(ctx, tenantA, deviceID, keyA); e != nil {
		t.Fatal(e)
	}
	if e = provisioner.Enroll(ctx, tenantB, deviceID, keyB); e != nil {
		t.Fatal(e)
	}
	// Runtime should only see its own tenant data
	lat, lon := 0.0, 0.0
	now := time.Now().UTC().Truncate(time.Microsecond)
	pos := ingest.Position{FixTime: now, Latitude: &lat, Longitude: &lon}
	if ok, e := runtime.Accept(ctx, keyA, pos, now); e != nil || !ok {
		t.Fatalf("tenant A accept: %v %v", ok, e)
	}
	if _, e = runtime.Latest(ctx, keyB); !errors.Is(e, ingest.ErrNotFound) {
		t.Fatalf("tenant B sees A: %v", e)
	}
	if ok, e := runtime.Accept(ctx, keyB, pos, now); e != nil || !ok {
		t.Fatalf("tenant B accept: %v %v", ok, e)
	}
	if r, e := runtime.Latest(ctx, keyA); e != nil || r.TenantID != tenantA {
		t.Fatalf("tenant A isolation: %v", e)
	}
	if r, e := runtime.Latest(ctx, keyB); e != nil || r.TenantID != tenantB {
		t.Fatalf("tenant B isolation: %v", e)
	}
}

func TestPostgresRuntimeSchemaRejectsInheritedCreate(t *testing.T) {
	migratorDSN := os.Getenv("ZALGPS_MIGRATOR_DATABASE_URL")
	runtimeDSN := os.Getenv("ZALGPS_RUNTIME_DATABASE_URL")
	provisionerDSN := os.Getenv("ZALGPS_PROVISIONER_DATABASE_URL")
	if migratorDSN == "" || runtimeDSN == "" || provisionerDSN == "" {
		if os.Getenv("ZALGPS_REQUIRE_ROLE_TESTS") == "1" {
			t.Fatal("required role-separated integration database not configured")
		}
		t.Skip("disposable role-separated integration database not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, migratorDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	runtime, err := Open(ctx, runtimeDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	provisioner, err := pgxpool.New(ctx, provisionerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer provisioner.Close()
	if err = runtime.CheckRuntimeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	var publicCreate, runtimeCreate, provisionerCreate bool
	err = runtime.pool.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM pg_catalog.pg_namespace n CROSS JOIN LATERAL pg_catalog.aclexplode(n.nspacl) a WHERE n.nspname='zalgps_v2' AND a.grantee=0 AND a.privilege_type='CREATE'),
		pg_catalog.has_schema_privilege('zalgps_runtime','zalgps_v2','CREATE'),
		pg_catalog.has_schema_privilege('zalgps_provisioner','zalgps_v2','CREATE')`).Scan(&publicCreate, &runtimeCreate, &provisionerCreate)
	if err != nil || publicCreate || runtimeCreate || provisionerCreate {
		t.Fatalf("unexpected schema CREATE privileges: %v", err)
	}
	var roleMember bool
	err = runtime.pool.QueryRow(ctx, `SELECT pg_catalog.pg_has_role('zalgps_runtime','zalgps_functions','MEMBER') OR pg_catalog.pg_has_role('zalgps_provisioner','zalgps_functions','MEMBER') OR pg_catalog.pg_has_role('zalgps_runtime','zalgps_migrator','MEMBER') OR pg_catalog.pg_has_role('zalgps_provisioner','zalgps_migrator','MEMBER')`).Scan(&roleMember)
	if err != nil || roleMember {
		t.Fatalf("application role inherits privileged role: %v", err)
	}
	for _, role := range []struct {
		name string
		pool *pgxpool.Pool
	}{{"runtime", runtime.pool}, {"provisioner", provisioner}} {
		var tableAccess, privateFunction bool
		err = role.pool.QueryRow(ctx, `SELECT
			pg_catalog.has_table_privilege(current_user,'zalgps_v2.devices','SELECT') OR pg_catalog.has_table_privilege(current_user,'zalgps_v2.devices','INSERT') OR pg_catalog.has_table_privilege(current_user,'zalgps_v2.devices','UPDATE') OR pg_catalog.has_table_privilege(current_user,'zalgps_v2.devices','DELETE'),
			pg_catalog.has_function_privilege(current_user,'zalgps_v2.authenticate_device(bytea)','EXECUTE')`).Scan(&tableAccess, &privateFunction)
		if err != nil || tableAccess || privateFunction {
			t.Fatalf("%s has direct table access or private function EXECUTE: %v", role.name, err)
		}
	}
	const group = "zalgps_test_schema_create"
	// The disposable cluster bootstrap pre-creates this unprivileged group
	// and grants membership to runtime. Migrator needs no CREATEROLE.
	defer func() {
		if _, err := admin.Exec(context.Background(), "REVOKE CREATE ON SCHEMA zalgps_v2 FROM "+group); err != nil {
			t.Errorf("revoke test schema grant: %v", err)
		}
	}()
	if _, err = admin.Exec(ctx, "GRANT CREATE ON SCHEMA zalgps_v2 TO "+group); err != nil {
		t.Fatal(err)
	}
	if err = runtime.CheckRuntimeSchema(ctx); err == nil {
		t.Fatal("runtime schema accepted inherited CREATE privilege")
	}
}
