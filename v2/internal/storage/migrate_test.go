package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
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
	return migrationDBWithTemplate(t, "template1", true)
}

func migrationDBWithTemplate(t *testing.T, template string, requirePostGIS bool) (context.Context, *Postgres) {
	t.Helper()
	dsn := os.Getenv("ZALGPS_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("ZALGPS_REQUIRE_ROLE_TESTS") == "1" {
			t.Fatal("required disposable integration database not configured")
		}
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
	if _, e = admin.Exec(ctx, "CREATE DATABASE "+ident+" TEMPLATE "+pgx.Identifier{template}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := admin.Exec(c, "DROP DATABASE "+ident); e != nil {
			t.Errorf("cleanup test database: %v", e)
		}
	})
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
	var postgis bool
	if e = p.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_extension WHERE extname='postgis')").Scan(&postgis); e != nil || postgis != requirePostGIS {
		t.Fatal("disposable test DB PostGIS prerequisite differs from expected template")
	}
	return ctx, p
}

func TestPostGISMigrationUpgradeBackfill(t *testing.T) {
	ctx, p := migrationDB(t)
	ms, e := migrations()
	if e != nil {
		t.Fatal(e)
	}
	if len(ms) < 3 {
		t.Fatalf("expected at least three migrations, got %d", len(ms))
	}
	for _, m := range ms[:2] {
		if _, e = p.pool.Exec(ctx, m.sql, pgx.QueryExecModeSimpleProtocol); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = p.pool.Exec(ctx, ledgerSQL); e != nil {
		t.Fatal(e)
	}
	for _, m := range ms[:2] {
		if _, e = p.pool.Exec(ctx, "INSERT INTO zalgps_v2.schema_migrations(version,name,checksum) VALUES($1,$2,$3)", m.version, m.name, m.checksum); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = p.pool.Exec(ctx, `INSERT INTO zalgps_v2.tenants(id) VALUES('db03-backfill-test');
	 INSERT INTO zalgps_v2.devices(tenant_id,device_id,token_hash) VALUES('db03-backfill-test','device',decode(repeat('01',32),'hex'));
	 INSERT INTO zalgps_v2.positions(tenant_id,device_id,fingerprint,fix_time,received_at,latitude,longitude)
	 VALUES('db03-backfill-test','device',decode(repeat('02',32),'hex'),clock_timestamp(),clock_timestamp(),-45.25,12.5)`); e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	if e = p.CheckSchema(ctx); e != nil {
		t.Fatalf("upgraded schema: %v", e)
	}
	var method, opclass, indexedColumn string
	var keyCount int
	if e = p.pool.QueryRow(ctx, `SELECT am.amname, opc.opcname, a.attname, i.indnkeyatts
	 FROM pg_catalog.pg_index i
	 JOIN pg_catalog.pg_class idx ON idx.oid=i.indexrelid
	 JOIN pg_catalog.pg_am am ON am.oid=idx.relam
	 JOIN pg_catalog.pg_opclass opc ON opc.oid=i.indclass[0]
	 JOIN pg_catalog.pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=i.indkey[0]
	 WHERE idx.oid='zalgps_v2.positions_location_gist_idx'::pg_catalog.regclass AND i.indisvalid AND i.indisready AND i.indpred IS NULL AND i.indexprs IS NULL AND i.indrelid='zalgps_v2.positions'::pg_catalog.regclass`).Scan(&method, &opclass, &indexedColumn, &keyCount); e != nil || method != "gist" || opclass != "gist_geography_ops" || indexedColumn != "location" || keyCount != 1 {
		t.Fatalf("location index: method=%q opclass=%q column=%q keys=%d err=%v", method, opclass, indexedColumn, keyCount, e)
	}
	var distance float64
	var within1120, within1100 bool
	if e = p.pool.QueryRow(ctx, `WITH points AS (
	 SELECT public.ST_SetSRID(public.ST_MakePoint(0,0),4326)::public.geography AS p1,
	        public.ST_SetSRID(public.ST_MakePoint(0.01,0),4326)::public.geography AS p2
	)
	SELECT public.ST_Distance(p1,p2), public.ST_DWithin(p1,p2,1120), public.ST_DWithin(p1,p2,1100)
	FROM points`).Scan(&distance, &within1120, &within1100); e != nil || distance < 1110 || distance > 1120 || !within1120 || within1100 {
		t.Fatalf("geography distance units: distance=%v within1120=%v within1100=%v err=%v", distance, within1120, within1100, e)
	}
	if _, e = p.pool.Exec(ctx, `INSERT INTO zalgps_v2.positions(tenant_id,device_id,fingerprint,fix_time,received_at,latitude,longitude)
	 VALUES('db03-backfill-test','device',decode(repeat('03',32),'hex'),clock_timestamp(),clock_timestamp(),91,12.5)`); e == nil {
		t.Fatal("invalid latitude accepted")
	}
	if _, e = p.pool.Exec(ctx, `INSERT INTO zalgps_v2.positions(tenant_id,device_id,fingerprint,fix_time,received_at,latitude,longitude)
	 VALUES('db03-backfill-test','device',decode(repeat('04',32),'hex'),clock_timestamp(),clock_timestamp(),0,181)`); e == nil {
		t.Fatal("invalid longitude accepted")
	}
	var x, y, lat, lon float64
	var srid int
	if e = p.pool.QueryRow(ctx, `SELECT public.ST_X(location::public.geometry), public.ST_Y(location::public.geometry), public.ST_SRID(location::public.geometry), latitude, longitude
	 FROM zalgps_v2.positions WHERE tenant_id='db03-backfill-test' AND device_id='device'`).Scan(&x, &y, &srid, &lat, &lon); e != nil || x != 12.5 || y != -45.25 || srid != 4326 || lat != y || lon != x {
		t.Fatalf("backfilled point/scalars: point=(%v,%v) SRID=%d scalars=(%v,%v) err=%v", x, y, srid, lat, lon, e)
	}
}

func TestPostGISRequiredWithoutPartialMigration(t *testing.T) {
	ctx, p := migrationDBWithTemplate(t, "template0", false)
	if err := p.Migrate(ctx); err == nil || !strings.Contains(err.Error(), "DB-03 requires PostGIS") {
		t.Fatalf("expected explicit missing PostGIS rejection: %v", err)
	}
	var exists bool
	if err := p.pool.QueryRow(ctx, "SELECT to_regnamespace('zalgps_v2') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("failed migration left partial schema")
	}
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
