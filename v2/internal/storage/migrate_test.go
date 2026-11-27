package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test uses a database created solely for this test and drops only that
// randomly named database after all connections are closed.
func migrationDB(t *testing.T) (context.Context, *Postgres) {
	t.Helper()
	dsn := os.Getenv("ZALGPS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable integration database not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	cfg, e := pgx.ParseConfig(dsn)
	if e != nil {
		t.Fatal("invalid test DSN")
	}
	admin, e := pgx.ConnectConfig(ctx, cfg)
	if e != nil {
		t.Fatal("cannot connect to test DB")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	var b [8]byte
	if _, e = rand.Read(b[:]); e != nil {
		t.Fatal(e)
	}
	name := "zalgps_migration_test_" + hex.EncodeToString(b[:])
	ident := pgx.Identifier{name}.Sanitize()
	if _, e = admin.Exec(ctx, "CREATE DATABASE "+ident); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := admin.Exec(c, "DROP DATABASE "+ident); e != nil {
			t.Errorf("cleanup test database: %v", e)
		}
	})
	// Build a pool from the same parsed credentials rather than printing a DSN.
	poolCfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	poolCfg.ConnConfig.Database = name
	poolCfg.MaxConns = 4
	pool, e := pgxpool.NewWithConfig(ctx, poolCfg)
	if e != nil {
		t.Fatal(e)
	}
	p := &Postgres{pool}
	t.Cleanup(p.Close)
	return ctx, p
}

func TestVersionedMigrations(t *testing.T) {
	t.Run("fresh_concurrent_repeat", func(t *testing.T) {
		ctx, p := migrationDB(t)
		if e := p.CheckSchema(ctx); e == nil {
			t.Fatal("empty DB reported ready")
		}
		var wg sync.WaitGroup
		errs := make(chan error, 4)
		for range 4 {
			wg.Go(func() { errs <- p.Migrate(ctx) })
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		if e := p.CheckSchema(ctx); e != nil {
			t.Fatal(e)
		}
		ms, e := migrations()
		if e != nil {
			t.Fatal(e)
		}
		var n int
		if e := p.pool.QueryRow(ctx, "SELECT count(*) FROM zalgps_v2.schema_migrations").Scan(&n); e != nil || n != len(ms) {
			t.Fatalf("ledger count %d: %v", n, e)
		}
		if e := p.Migrate(ctx); e != nil {
			t.Fatal(e)
		}
		if _, e := p.pool.Exec(ctx, "UPDATE zalgps_v2.schema_migrations SET checksum=repeat('0',64)"); e != nil {
			t.Fatal(e)
		}
		if e := p.CheckSchema(ctx); e == nil {
			t.Fatal("corrupted checksum accepted")
		}
		if e := p.Migrate(ctx); e == nil {
			t.Fatal("corrupted checksum migrated")
		}
	})
	t.Run("adopt_pilot_preserves_rows", func(t *testing.T) {
		ctx, p := migrationDB(t)
		ms, e := migrations()
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.pool.Exec(ctx, ms[0].sql, pgx.QueryExecModeSimpleProtocol); e != nil {
			t.Fatal(e)
		}
		if _, e = p.pool.Exec(ctx, "INSERT INTO zalgps_v2.tenants(id) VALUES('synthetic')"); e != nil {
			t.Fatal(e)
		}
		if e = p.Migrate(ctx); e != nil {
			t.Fatal(e)
		}
		var n int
		if e = p.pool.QueryRow(ctx, "SELECT count(*) FROM zalgps_v2.tenants WHERE id='synthetic'").Scan(&n); e != nil || n != 1 {
			t.Fatal("pilot row lost")
		}
		if e = p.CheckSchema(ctx); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("refuse_drift_without_ledger", func(t *testing.T) {
		ctx, p := migrationDB(t)
		ms, e := migrations()
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.pool.Exec(ctx, ms[0].sql, pgx.QueryExecModeSimpleProtocol); e != nil {
			t.Fatal(e)
		}
		if _, e = p.pool.Exec(ctx, "ALTER TABLE zalgps_v2.devices ADD COLUMN unexpected text"); e != nil {
			t.Fatal(e)
		}
		if e = p.Migrate(ctx); e == nil {
			t.Fatal("drift adopted")
		}
		var exists bool
		if e = p.pool.QueryRow(ctx, "SELECT to_regclass('zalgps_v2.schema_migrations') IS NOT NULL").Scan(&exists); e != nil || exists {
			t.Fatal("failed migration left ledger")
		}
	})
	t.Run("refuse_missing_core_relation", func(t *testing.T) {
		ctx, p := migrationDB(t)
		if e := p.Migrate(ctx); e != nil {
			t.Fatal(e)
		}
		if _, e := p.pool.Exec(ctx, "ALTER TABLE zalgps_v2.positions RENAME TO positions_unexpected"); e != nil {
			t.Fatal(e)
		}
		if e := p.CheckSchema(ctx); e == nil {
			t.Fatal("missing table accepted")
		}
		if e := p.Migrate(ctx); e == nil {
			t.Fatal("missing table silently accepted")
		}
	})
	t.Run("refuse_empty_existing_ledger", func(t *testing.T) {
		ctx, p := migrationDB(t)
		if e := p.Migrate(ctx); e != nil {
			t.Fatal(e)
		}
		if _, e := p.pool.Exec(ctx, "DELETE FROM zalgps_v2.schema_migrations"); e != nil {
			t.Fatal(e)
		}
		if e := p.Migrate(ctx); e == nil {
			t.Fatal("empty existing ledger accepted")
		}
	})

}
