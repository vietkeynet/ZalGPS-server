CREATE SCHEMA IF NOT EXISTS zalgps_v2;
CREATE TABLE IF NOT EXISTS zalgps_v2.tenants(id text PRIMARY KEY);
CREATE TABLE IF NOT EXISTS zalgps_v2.devices(
 tenant_id text NOT NULL REFERENCES zalgps_v2.tenants(id),
 device_id text NOT NULL,
 token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 revoked boolean NOT NULL DEFAULT false,
 PRIMARY KEY(tenant_id,device_id)
);
CREATE TABLE IF NOT EXISTS zalgps_v2.positions(
 tenant_id text NOT NULL,device_id text NOT NULL,
 fingerprint bytea NOT NULL CHECK(octet_length(fingerprint)=32),
 fix_time timestamptz NOT NULL,received_at timestamptz NOT NULL,
 latitude double precision NOT NULL CHECK(latitude BETWEEN -90 AND 90),
 longitude double precision NOT NULL CHECK(longitude BETWEEN -180 AND 180),
 speed_kph double precision CHECK(speed_kph BETWEEN 0 AND 1000),
 PRIMARY KEY(tenant_id,device_id,fingerprint),
 FOREIGN KEY(tenant_id,device_id) REFERENCES zalgps_v2.devices(tenant_id,device_id)
);
CREATE INDEX IF NOT EXISTS positions_latest_idx ON zalgps_v2.positions(tenant_id,device_id,fix_time DESC,fingerprint DESC);
