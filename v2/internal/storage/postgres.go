// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vietkeynet/ZalGPS-server/v2/internal/ingest"
	"regexp"
	"time"
)

var idPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")

type Postgres struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, dsn string) (*Postgres, error) {
	if dsn == "" {
		return nil, errors.New("database configuration required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.MaxConns = 8
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return nil, errors.New("database unavailable")
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, errors.New("database unavailable")
	}
	return &Postgres{p}, nil
}
func (p *Postgres) Close() { p.pool.Close() }
func (p *Postgres) Enroll(ctx context.Context, tenant, device string, key [32]byte) error {
	if !idPattern.MatchString(tenant) || !idPattern.MatchString(device) {
		return errors.New("invalid identifier")
	}
	_, err := p.pool.Exec(ctx, "SELECT zalgps_v2.enroll_device($1, $2, $3)", tenant, device, key[:])
	return provisionerError(err)
}

func (p *Postgres) Rotate(ctx context.Context, tenant, device string, key [32]byte) error {
	if !idPattern.MatchString(tenant) || !idPattern.MatchString(device) {
		return errors.New("invalid identifier")
	}
	_, err := p.pool.Exec(ctx, "SELECT zalgps_v2.rotate_device($1, $2, $3)", tenant, device, key[:])
	return provisionerError(err)
}

func (p *Postgres) Revoke(ctx context.Context, tenant, device string) error {
	if !idPattern.MatchString(tenant) || !idPattern.MatchString(device) {
		return errors.New("invalid identifier")
	}
	_, err := p.pool.Exec(ctx, "SELECT zalgps_v2.revoke_device($1, $2)", tenant, device)
	return provisionerError(err)
}

func provisionerError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Message == "device not found" {
		return ingest.ErrNotFound
	}
	return err
}

func authenticationError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Message == "device authentication failed" {
		return ingest.ErrUnauthorized
	}
	return err
}

func (p *Postgres) Accept(ctx context.Context, key [32]byte, pos ingest.Position, received time.Time) (bool, error) {
	if pos.Latitude == nil || pos.Longitude == nil {
		return false, errors.New("missing position")
	}
	b, err := json.Marshal(pos)
	if err != nil {
		return false, err
	}
	fingerprint := sha256.Sum256(b)
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.Background())
	var inserted bool
	err = tx.QueryRow(ctx, "SELECT zalgps_v2.accept_position($1,$2,$3,$4,$5,$6,$7)", key[:], pos.FixTime, received, fingerprint[:], *pos.Latitude, *pos.Longitude, pos.SpeedKPH).Scan(&inserted)
	if err != nil {
		return false, authenticationError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return inserted, nil
}

func (p *Postgres) Latest(ctx context.Context, key [32]byte) (ingest.Record, error) {
	var record ingest.Record
	err := p.pool.QueryRow(ctx, "SELECT fix_time,received_at,latitude,longitude,speed_kph,tenant_id,device_id FROM zalgps_v2.latest_position($1)", key[:]).Scan(&record.FixTime, &record.ReceivedAt, &record.Latitude, &record.Longitude, &record.SpeedKPH, &record.TenantID, &record.DeviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return record, ingest.ErrNotFound
	}
	if err != nil {
		return record, authenticationError(err)
	}
	record.FixTime = record.FixTime.UTC()
	record.ReceivedAt = record.ReceivedAt.UTC()
	return record, nil
}

func (p *Postgres) CheckRuntimeSchema(ctx context.Context) error {
	if err := p.CheckSchema(ctx); err != nil {
		return err
	}
	var valid bool
	err := p.pool.QueryRow(ctx, `SELECT
		current_user='zalgps_runtime' AND session_user='zalgps_runtime'
		AND EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=current_user AND NOT (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication))
		AND (SELECT count(*) = 2 FROM pg_catalog.pg_proc f JOIN pg_catalog.pg_namespace n ON n.oid=f.pronamespace
		 WHERE n.nspname='zalgps_v2' AND (f.proname,f.proargtypes::text) IN (
		 ('accept_position','17 1184 1184 17 701 701 701'),('latest_position','17'))
		 AND f.prosecdef AND pg_catalog.pg_get_userbyid(f.proowner)='zalgps_functions'
		 AND 'search_path=pg_catalog, zalgps_v2, pg_temp' = ANY(f.proconfig)
		 AND pg_catalog.has_function_privilege(current_user, f.oid, 'EXECUTE')
		 AND f.proacl IS NOT NULL AND NOT EXISTS (
		   SELECT 1 FROM pg_catalog.aclexplode(f.proacl) a WHERE a.grantee=0 AND a.privilege_type='EXECUTE'))
		AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_proc f JOIN pg_catalog.pg_namespace n ON n.oid=f.pronamespace
		 WHERE n.nspname='zalgps_v2' AND f.proname IN ('authenticate_device','enroll_device','rotate_device','revoke_device')
		 AND pg_catalog.has_function_privilege(current_user, f.oid, 'EXECUTE'))
		AND (SELECT count(*)=3 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		 WHERE n.nspname='zalgps_v2' AND c.relname IN ('tenants','devices','positions') AND c.relrowsecurity AND c.relforcerowsecurity)
		AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		 WHERE n.nspname='zalgps_v2' AND c.relname IN ('tenants','devices','positions')
		 AND (pg_catalog.has_table_privilege(current_user,c.oid,'SELECT') OR pg_catalog.has_table_privilege(current_user,c.oid,'INSERT')
		 OR pg_catalog.has_table_privilege(current_user,c.oid,'UPDATE') OR pg_catalog.has_table_privilege(current_user,c.oid,'DELETE')
		 OR pg_catalog.has_table_privilege(current_user,c.oid,'TRUNCATE') OR pg_catalog.has_table_privilege(current_user,c.oid,'REFERENCES')
		 OR pg_catalog.has_table_privilege(current_user,c.oid,'TRIGGER')
		 OR pg_catalog.has_any_column_privilege(current_user,c.oid,'SELECT,INSERT,UPDATE,REFERENCES')))
		AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles r WHERE r.rolname IN ('zalgps_migrator','zalgps_functions','zalgps_provisioner')
		 AND pg_catalog.pg_has_role(current_user,r.oid,'MEMBER'))
		AND EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname='zalgps_functions' AND NOT (rolcanlogin OR rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication))
		AND NOT pg_catalog.has_schema_privilege(current_user,'zalgps_v2','CREATE')`).Scan(&valid)
	if err != nil || !valid {
		return errors.New("database schema not ready")
	}
	return nil
}
