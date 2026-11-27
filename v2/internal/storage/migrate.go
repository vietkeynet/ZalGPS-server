// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version             int
	name, sql, checksum string
}

func migrations() ([]migration, error) {
	entries, e := fs.ReadDir(migrationFiles, "migrations")
	if e != nil {
		return nil, e
	}
	result := make([]migration, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		prefix, _, ok := strings.Cut(name, "_")
		v, e := strconv.Atoi(prefix)
		if entry.IsDir() || !ok || e != nil || len(prefix) != 4 || v != len(result)+1 || !strings.HasSuffix(name, ".sql") {
			return nil, errors.New("invalid migration sequence")
		}
		b, e := migrationFiles.ReadFile("migrations/" + name)
		if e != nil {
			return nil, e
		}
		h := sha256.Sum256(b)
		result = append(result, migration{v, name, string(b), hex.EncodeToString(h[:])})
	}
	if len(result) == 0 {
		return nil, errors.New("no migrations")
	}
	return result, nil
}

const ledgerSQL = `CREATE TABLE zalgps_v2.schema_migrations(
 version integer PRIMARY KEY CHECK(version>0),
 name text NOT NULL,
 checksum text NOT NULL CHECK(length(checksum)=64),
 applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
)`

type migrationQuery interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func applied(ctx context.Context, q migrationQuery, ms []migration) (int, error) {
	rows, e := q.Query(ctx, "SELECT version,name,checksum FROM zalgps_v2.schema_migrations ORDER BY version")
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if e = rows.Scan(&version, &name, &checksum); e != nil {
			return 0, e
		}
		if count >= len(ms) || version != ms[count].version || name != ms[count].name || checksum != ms[count].checksum {
			return 0, errors.New("migration history mismatch")
		}
		count++
	}
	return count, rows.Err()
}

// Migrate applies immutable migrations under a transaction-scoped advisory lock.
// Existing pre-ledger pilot databases are adopted only after exact structural validation.
func (p *Postgres) Migrate(ctx context.Context) error {
	ms, e := migrations()
	if e != nil {
		return e
	}
	tx, e := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(82460121)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SET LOCAL search_path TO pg_catalog"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "SET LOCAL synchronous_commit TO on"); e != nil {
		return e
	}
	var ledger bool
	if e = tx.QueryRow(ctx, "SELECT to_regclass('zalgps_v2.schema_migrations') IS NOT NULL").Scan(&ledger); e != nil {
		return e
	}
	if !ledger {
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='zalgps_v2'`).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			if e = validatePilot(ctx, tx, ms[0]); e != nil {
				return fmt.Errorf("pilot schema adoption refused: %w", e)
			}
		}
		if _, e = tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS zalgps_v2"); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, ledgerSQL); e != nil {
			return e
		}
	}
	n, e := applied(ctx, tx, ms)
	if e != nil {
		return e
	}
	if ledger && n == 0 {
		return errors.New("existing migration ledger is empty")
	}
	for _, m := range ms[n:] {
		if _, e = tx.Exec(ctx, m.sql, pgx.QueryExecModeSimpleProtocol); e != nil {
			return fmt.Errorf("migration %d failed: %w", m.version, e)
		}
		if _, e = tx.Exec(ctx, "INSERT INTO zalgps_v2.schema_migrations(version,name,checksum) VALUES($1,$2,$3)", m.version, m.name, m.checksum); e != nil {
			return e
		}
	}
	if e = checkCoreRelations(ctx, tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func (p *Postgres) CheckSchema(ctx context.Context) error {
	ms, e := migrations()
	if e != nil {
		return e
	}
	n, e := applied(ctx, p.pool, ms)
	if e != nil || n != len(ms) {
		return errors.New("database schema not ready or migration history mismatch")
	}
	return checkCoreRelations(ctx, p.pool)
}

type migrationRowQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func checkCoreRelations(ctx context.Context, q migrationRowQuery) error {
	var ok bool
	e := q.QueryRow(ctx, "SELECT pg_catalog.to_regclass('zalgps_v2.tenants') IS NOT NULL AND pg_catalog.to_regclass('zalgps_v2.devices') IS NOT NULL AND pg_catalog.to_regclass('zalgps_v2.positions') IS NOT NULL").Scan(&ok)
	if e != nil || !ok {
		return errors.New("required database relations missing")
	}
	return nil
}

// Compare catalog structure to a reference schema constructed in a savepoint.
// The generated schema never survives the transaction, and no existing objects
// are dropped. A conservative mismatch requires manual review, not silent repair.
func validatePilot(ctx context.Context, tx pgx.Tx, m migration) error {
	var b [12]byte
	if _, e := rand.Read(b[:]); e != nil {
		return e
	}
	ref := "zalgps_v2_ref_" + hex.EncodeToString(b[:])
	if _, e := tx.Exec(ctx, "SAVEPOINT pilot_reference"); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, strings.ReplaceAll(m.sql, "zalgps_v2", ref), pgx.QueryExecModeSimpleProtocol); e != nil {
		return e
	}
	actual, e := schemaSignature(ctx, tx, "zalgps_v2")
	if e != nil {
		return e
	}
	expected, e := schemaSignature(ctx, tx, ref)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT pilot_reference"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "RELEASE SAVEPOINT pilot_reference"); e != nil {
		return e
	}
	if actual != expected {
		return errors.New("existing schema differs from immutable pilot migration")
	}
	return nil
}

func schemaSignature(ctx context.Context, tx pgx.Tx, namespace string) (string, error) {
	// Column order/type/default/nullability, constraints, indexes and relation kinds.
	// Compare under the same PG version and search_path. Owners/grants intentionally
	// remain a separate DB-02 gate. Extra relations/triggers/policies fail adoption.
	const query = `
 SELECT kind, detail FROM (
 SELECT 'relation' AS kind,jsonb_build_array(c.relname,c.relkind,c.relpersistence,c.relrowsecurity,c.relforcerowsecurity)::text AS detail
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1
 UNION ALL
 SELECT 'column',jsonb_build_array(c.relname,a.attnum,a.attname,format_type(a.atttypid,a.atttypmod),a.attnotnull,a.attidentity,a.attgenerated,coalesce(pg_get_expr(d.adbin,d.adrelid),''))::text
 FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace
 LEFT JOIN pg_attrdef d ON d.adrelid=c.oid AND d.adnum=a.attnum
 WHERE n.nspname=$1 AND c.relkind IN ('r','p') AND a.attnum>0 AND NOT a.attisdropped
 UNION ALL
 SELECT 'constraint',jsonb_build_array(c.relname,k.conname,k.convalidated,pg_get_constraintdef(k.oid))::text
 FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1
 UNION ALL
 SELECT 'index',jsonb_build_array(c.relname,i.indisvalid,i.indisready,pg_get_indexdef(i.indexrelid))::text
 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1
 UNION ALL
 SELECT 'trigger',jsonb_build_array(c.relname,t.tgname,pg_get_triggerdef(t.oid))::text
 FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND NOT t.tgisinternal
 UNION ALL
 SELECT 'policy',jsonb_build_array(c.relname,p.polname)::text
 FROM pg_policy p JOIN pg_class c ON c.oid=p.polrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1
 ) s ORDER BY kind,detail`
	rows, e := tx.Query(ctx, query, namespace)
	if e != nil {
		return "", e
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var kind, detail string
		if e = rows.Scan(&kind, &detail); e != nil {
			return "", e
		}
		lines = append(lines, kind+":"+strings.ReplaceAll(detail, namespace+".", "zalgps_v2."))
	}
	if e = rows.Err(); e != nil {
		return "", e
	}
	return strings.Join(lines, "\n"), nil
}
