package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vietkeynet/ZalGPS-server/v2/internal/ingest"
)

func TestPostgresPoolAndRevocationBoundary(t *testing.T) {
	runtimeDSN := os.Getenv("ZALGPS_RUNTIME_DATABASE_URL")
	provisionerDSN := os.Getenv("ZALGPS_PROVISIONER_DATABASE_URL")
	migratorDSN := os.Getenv("ZALGPS_MIGRATOR_DATABASE_URL")
	if runtimeDSN == "" || provisionerDSN == "" || migratorDSN == "" {
		if os.Getenv("ZALGPS_REQUIRE_ROLE_TESTS") == "1" {
			t.Fatal("role integration configuration required")
		}
		t.Skip("disposable role integration configuration required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrator, err := Open(ctx, migratorDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	if err = migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = migrator.CheckRuntimeSchema(ctx); err == nil {
		t.Fatal("migrator accepted as runtime")
	}

	cfg, err := pgxpool.ParseConfig(runtimeDSN)
	if err != nil {
		t.Fatal("invalid runtime test DSN")
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &Postgres{pool}
	defer runtime.Close()
	if err = runtime.CheckRuntimeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	provisioner, err := Open(ctx, provisionerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer provisioner.Close()
	if err = provisioner.CheckRuntimeSchema(ctx); err == nil {
		t.Fatal("provisioner accepted as runtime")
	}

	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	a, b := "pool-a-"+hex.EncodeToString(suffix[:]), "pool-b-"+hex.EncodeToString(suffix[:])
	ka, kb, unknown := randomKey(t), randomKey(t), randomKey(t)
	if err = provisioner.Enroll(ctx, a, "same", ka); err != nil {
		t.Fatal(err)
	}
	if err = provisioner.Enroll(ctx, b, "same", kb); err != nil {
		t.Fatal(err)
	}

	var pidBefore, pidAfter int
	if err = pool.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pidBefore); err != nil {
		t.Fatal(err)
	}
	// Deliberately leave caller-controlled session state in the only connection.
	if _, err = pool.Exec(ctx, "SELECT set_config('zalgps.tenant_id',$1,false), set_config('zalgps.device_id','same',false)", b); err != nil {
		t.Fatal(err)
	}
	lat, lon := 12.0, 104.0
	now := time.Now().UTC().Truncate(time.Microsecond)
	pos := ingest.Position{FixTime: now, Latitude: &lat, Longitude: &lon}
	if inserted, e := runtime.Accept(ctx, ka, pos, now); e != nil || !inserted {
		t.Fatalf("tenant A insert: %v %v", inserted, e)
	}
	if _, e := runtime.Latest(ctx, kb); !errors.Is(e, ingest.ErrNotFound) {
		t.Fatalf("caller context leaked tenant A: %v", e)
	}
	if _, e := runtime.Latest(ctx, unknown); !errors.Is(e, ingest.ErrUnauthorized) {
		t.Fatalf("unknown token: %v", e)
	}
	if inserted, e := runtime.Accept(ctx, kb, pos, now); e != nil || !inserted {
		t.Fatalf("tenant B insert: %v %v", inserted, e)
	}
	for _, x := range []struct {
		key    [32]byte
		tenant string
	}{{ka, a}, {kb, b}, {ka, a}} {
		r, e := runtime.Latest(ctx, x.key)
		if e != nil || r.TenantID != x.tenant {
			t.Fatalf("reused connection tenant mismatch: %v", e)
		}
	}
	if err = pool.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pidAfter); err != nil || pidBefore != pidAfter {
		t.Fatal("pool reuse not demonstrated")
	}

	for _, query := range []string{
		"SELECT * FROM zalgps_v2.tenants",
		"SELECT * FROM zalgps_v2.positions",
		"UPDATE zalgps_v2.schema_migrations SET checksum=repeat('0',64)",
		"SET ROLE zalgps_functions",
		"SET ROLE zalgps_migrator",
	} {
		// Always roll back, including when a security regression unexpectedly allows SQL.
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, query)
		_ = tx.Rollback(ctx)
		var pe *pgconn.PgError
		if !errors.As(e, &pe) || pe.Code != "42501" {
			t.Fatal("runtime permission boundary did not return insufficient privilege")
		}
	}

	// Hold the token's FOR SHARE lock in an uncommitted ingest. Revocation must
	// not pass that lock; lock_timeout makes the assertion deterministic.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	fp := randomKey(t)
	var inserted bool
	err = tx.QueryRow(ctx, "SELECT zalgps_v2.accept_position($1,$2,$3,$4,$5,$6,$7)", ka[:], now.Add(time.Second), now, fp[:], lat, lon, nil).Scan(&inserted)
	if err != nil || !inserted {
		t.Fatalf("locked ingest: %v %v", inserted, err)
	}
	ptx, err := provisioner.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ptx.Rollback(context.Background())
	if _, err = ptx.Exec(ctx, "SET LOCAL lock_timeout='150ms'"); err != nil {
		t.Fatal(err)
	}
	_, err = ptx.Exec(ctx, "SELECT zalgps_v2.revoke_device($1,$2)", a, "same")
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "55P03" {
		t.Fatal("revocation did not serialize with ingest lock")
	}
	if err = ptx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = provisioner.Revoke(ctx, a, "same"); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Accept(ctx, ka, pos, now); !errors.Is(err, ingest.ErrUnauthorized) {
		t.Fatal("revoked token accepted after commit")
	}
	if r, e := runtime.Latest(ctx, kb); e != nil || r.TenantID != b {
		t.Fatal("other tenant affected by revocation")
	}
}

func TestMigrationFromVersionOneLedger(t *testing.T) {
	ctx, p := migrationDB(t)
	ms, err := migrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) < 2 {
		t.Fatal("role migration missing")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, ms[0].sql, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, ledgerSQL); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO zalgps_v2.schema_migrations(version,name,checksum) VALUES($1,$2,$3)", ms[0].version, ms[0].name, ms[0].checksum); err != nil {
		t.Fatal(err)
	}
	key := randomKey(t)
	if _, err = tx.Exec(ctx, "INSERT INTO zalgps_v2.tenants(id) VALUES('upgrade-test')"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO zalgps_v2.devices(tenant_id,device_id,token_hash) VALUES('upgrade-test','device',$1)", key[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO zalgps_v2.positions(tenant_id,device_id,fingerprint,fix_time,received_at,latitude,longitude) VALUES('upgrade-test','device',$1,now(),now(),12,104)", key[:]); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = p.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = p.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = p.pool.QueryRow(ctx, "SELECT count(*) FROM zalgps_v2.positions WHERE tenant_id='upgrade-test' AND device_id='device' AND latitude=12 AND longitude=104").Scan(&n); err != nil || n != 1 {
		t.Fatal("version-one position not preserved")
	}
	var checksum string
	if err = p.pool.QueryRow(ctx, "SELECT checksum FROM zalgps_v2.schema_migrations WHERE version=1").Scan(&checksum); err != nil || checksum != ms[0].checksum {
		t.Fatal("version-one ledger changed")
	}
}
