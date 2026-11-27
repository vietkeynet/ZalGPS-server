// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vietkeynet/ZalGPS-server/v2/internal/ingest"
	"regexp"
	"time"
)

//go:embed schema.sql
var schema string
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
func (p *Postgres) Migrate(ctx context.Context) error {
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(82460121)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, schema, pgx.QueryExecModeSimpleProtocol); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (p *Postgres) CheckSchema(ctx context.Context) error {
	var ok bool
	e := p.pool.QueryRow(ctx, "SELECT to_regclass('zalgps_v2.positions') IS NOT NULL AND to_regclass('zalgps_v2.devices') IS NOT NULL").Scan(&ok)
	if e != nil || !ok {
		return errors.New("database schema not ready")
	}
	return nil
}
func (p *Postgres) Enroll(ctx context.Context, tenant, device string, key [32]byte) error {
	if !idPattern.MatchString(tenant) || !idPattern.MatchString(device) {
		return errors.New("invalid identifier")
	}
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(ctx, "INSERT INTO zalgps_v2.tenants(id) VALUES($1) ON CONFLICT DO NOTHING", tenant); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO zalgps_v2.devices(tenant_id,device_id,token_hash) VALUES($1,$2,$3)", tenant, device, key[:]); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (p *Postgres) Rotate(ctx context.Context, t, d string, k [32]byte) error {
	tag, e := p.pool.Exec(ctx, "UPDATE zalgps_v2.devices SET token_hash=$3,revoked=false WHERE tenant_id=$1 AND device_id=$2", t, d, k[:])
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ingest.ErrNotFound
	}
	return nil
}
func (p *Postgres) Revoke(ctx context.Context, t, d string) error {
	tag, e := p.pool.Exec(ctx, "UPDATE zalgps_v2.devices SET revoked=true WHERE tenant_id=$1 AND device_id=$2", t, d)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ingest.ErrNotFound
	}
	return nil
}
func authenticate(ctx context.Context, tx pgx.Tx, k [32]byte) (t, d string, e error) {
	e = tx.QueryRow(ctx, "SELECT tenant_id,device_id FROM zalgps_v2.devices WHERE token_hash=$1 AND NOT revoked FOR SHARE", k[:]).Scan(&t, &d)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ingest.ErrUnauthorized
	}
	return
}
func (p *Postgres) Accept(ctx context.Context, k [32]byte, pos ingest.Position, received time.Time) (bool, error) {
	if pos.Latitude == nil || pos.Longitude == nil {
		return false, errors.New("missing position")
	}
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(context.Background())
	t, d, e := authenticate(ctx, tx, k)
	if e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, "SET LOCAL synchronous_commit = on"); e != nil {
		return false, e
	}
	b, e := json.Marshal(pos)
	if e != nil {
		return false, e
	}
	hash := sha256.Sum256(b)
	tag, e := tx.Exec(ctx, `INSERT INTO zalgps_v2.positions(tenant_id,device_id,fingerprint,fix_time,received_at,latitude,longitude,speed_kph) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(tenant_id,device_id,fingerprint) DO NOTHING`, t, d, hash[:], pos.FixTime, received, *pos.Latitude, *pos.Longitude, pos.SpeedKPH)
	if e != nil {
		return false, e
	}
	if e = tx.Commit(ctx); e != nil {
		return false, e
	}
	return tag.RowsAffected() == 1, nil
}
func (p *Postgres) Latest(ctx context.Context, k [32]byte) (ingest.Record, error) {
	var r ingest.Record
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return r, e
	}
	defer tx.Rollback(context.Background())
	r.TenantID, r.DeviceID, e = authenticate(ctx, tx, k)
	if e != nil {
		return r, e
	}
	e = tx.QueryRow(ctx, `SELECT fix_time,received_at,latitude,longitude,speed_kph FROM zalgps_v2.positions WHERE tenant_id=$1 AND device_id=$2 ORDER BY fix_time DESC,fingerprint DESC LIMIT 1`, r.TenantID, r.DeviceID).Scan(&r.FixTime, &r.ReceivedAt, &r.Latitude, &r.Longitude, &r.SpeedKPH)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, ingest.ErrNotFound
	}
	if e != nil {
		return r, e
	}
	r.FixTime = r.FixTime.UTC()
	r.ReceivedAt = r.ReceivedAt.UTC()
	return r, tx.Commit(ctx)
}
