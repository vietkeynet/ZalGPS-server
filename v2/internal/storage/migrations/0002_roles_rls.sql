-- DB-02 role boundary: migrator owns tables; zalgps_functions owns definer functions.
-- Runtime and provisioner receive schema USAGE and function EXECUTE only, never table grants.
-- Cluster-wide zalgps_functions must be pre-provisioned as NOLOGIN and granted to migrator.
DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'zalgps_functions') THEN
  RAISE EXCEPTION 'DB-02 requires pre-provisioned cluster role zalgps_functions';
 END IF;
 IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'zalgps_functions' AND (rolcanlogin OR rolbypassrls)) THEN
  RAISE EXCEPTION 'DB-02 requires zalgps_functions with NOLOGIN and NOBYPASSRLS';
 END IF;
 IF current_user <> 'zalgps_migrator' THEN
  RAISE EXCEPTION 'DB-02 requires the dedicated migrator role';
 END IF;
 IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname IN ('zalgps_migrator','zalgps_functions','zalgps_runtime','zalgps_provisioner') AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolreplication)) THEN
  RAISE EXCEPTION 'DB-02 roles must not have privileged cluster attributes';
 END IF;
 IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname IN ('zalgps_runtime','zalgps_provisioner') AND (NOT rolcanlogin OR rolcreatedb)) THEN
  RAISE EXCEPTION 'DB-02 requires unprivileged application login roles';
 END IF;
 IF NOT pg_catalog.pg_has_role(current_user, 'zalgps_functions', 'MEMBER') THEN
  RAISE EXCEPTION 'DB-02 requires zalgps_migrator to be a member of zalgps_functions';
 END IF;
 IF pg_catalog.pg_has_role('zalgps_runtime', 'zalgps_functions', 'MEMBER')
    OR pg_catalog.pg_has_role('zalgps_provisioner', 'zalgps_functions', 'MEMBER')
    OR pg_catalog.pg_has_role('zalgps_runtime', 'zalgps_migrator', 'MEMBER')
    OR pg_catalog.pg_has_role('zalgps_provisioner', 'zalgps_migrator', 'MEMBER') THEN
  RAISE EXCEPTION 'DB-02 application roles must not be members of zalgps_functions or zalgps_migrator';
 END IF;
 IF EXISTS (SELECT 1 FROM pg_catalog.pg_namespace n CROSS JOIN LATERAL pg_catalog.aclexplode(n.nspacl) a
            WHERE n.oid = 'zalgps_v2'::pg_catalog.regnamespace AND a.grantee = 0 AND a.privilege_type = 'CREATE') THEN
  RAISE EXCEPTION 'DB-02 requires no PUBLIC CREATE on schema zalgps_v2';
 END IF;
 IF pg_catalog.has_schema_privilege('zalgps_runtime', 'zalgps_v2', 'CREATE')
    OR pg_catalog.has_schema_privilege('zalgps_provisioner', 'zalgps_v2', 'CREATE') THEN
  RAISE EXCEPTION 'DB-02 application roles must not have CREATE on schema zalgps_v2';
 END IF;
END
$$;
REVOKE CREATE ON SCHEMA zalgps_v2 FROM PUBLIC, zalgps_runtime, zalgps_provisioner;
REVOKE ALL PRIVILEGES ON TABLE zalgps_v2.tenants, zalgps_v2.devices, zalgps_v2.positions FROM PUBLIC, zalgps_runtime, zalgps_provisioner;
GRANT USAGE, CREATE ON SCHEMA zalgps_v2 TO zalgps_functions;
GRANT SELECT, INSERT, UPDATE ON zalgps_v2.devices TO zalgps_functions;
GRANT SELECT, INSERT ON zalgps_v2.positions TO zalgps_functions;
GRANT INSERT ON zalgps_v2.tenants TO zalgps_functions;
ALTER TABLE zalgps_v2.devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE zalgps_v2.devices FORCE ROW LEVEL SECURITY;
ALTER TABLE zalgps_v2.tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE zalgps_v2.tenants FORCE ROW LEVEL SECURITY;
ALTER TABLE zalgps_v2.positions ENABLE ROW LEVEL SECURITY;
ALTER TABLE zalgps_v2.positions FORCE ROW LEVEL SECURITY;

CREATE POLICY devices_functions_select ON zalgps_v2.devices FOR SELECT TO zalgps_functions USING (true);
CREATE POLICY devices_functions_insert ON zalgps_v2.devices FOR INSERT TO zalgps_functions WITH CHECK (true);
CREATE POLICY devices_functions_update ON zalgps_v2.devices FOR UPDATE TO zalgps_functions USING (true) WITH CHECK (true);
CREATE POLICY devices_migrator_all ON zalgps_v2.devices FOR ALL TO zalgps_migrator USING (true) WITH CHECK (true);
CREATE POLICY tenants_functions_write ON zalgps_v2.tenants FOR ALL TO zalgps_functions USING (true) WITH CHECK (true);
CREATE POLICY tenants_migrator_all ON zalgps_v2.tenants FOR ALL TO zalgps_migrator USING (true) WITH CHECK (true);
CREATE POLICY positions_functions_select ON zalgps_v2.positions FOR SELECT TO zalgps_functions USING (true);
CREATE POLICY positions_functions_insert ON zalgps_v2.positions FOR INSERT TO zalgps_functions WITH CHECK (true);

CREATE POLICY positions_migrator_all ON zalgps_v2.positions FOR ALL TO zalgps_migrator USING (true) WITH CHECK (true);

CREATE FUNCTION zalgps_v2.authenticate_device(p_token_hash bytea)
RETURNS TABLE(tenant_id text, device_id text)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, zalgps_v2, pg_temp
AS $$
BEGIN
 RETURN QUERY SELECT d.tenant_id,d.device_id FROM zalgps_v2.devices d WHERE d.token_hash=p_token_hash AND NOT d.revoked FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'device authentication failed'; END IF;
END
$$;
ALTER FUNCTION zalgps_v2.authenticate_device(bytea) OWNER TO zalgps_functions;
REVOKE ALL ON FUNCTION zalgps_v2.authenticate_device(bytea) FROM PUBLIC;

CREATE FUNCTION zalgps_v2.accept_position(p_token_hash bytea,p_fix_time timestamptz,p_received_at timestamptz,p_fingerprint bytea,p_latitude double precision,p_longitude double precision,p_speed_kph double precision)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, zalgps_v2, pg_temp
AS $$
DECLARE v_tenant text; v_device text; v_inserted integer;
BEGIN
 SELECT a.tenant_id,a.device_id INTO v_tenant,v_device FROM zalgps_v2.authenticate_device(p_token_hash) a;
 SET LOCAL synchronous_commit = on;
 INSERT INTO zalgps_v2.positions(tenant_id,device_id,fingerprint,fix_time,received_at,latitude,longitude,speed_kph)
 VALUES(v_tenant,v_device,p_fingerprint,p_fix_time,p_received_at,p_latitude,p_longitude,p_speed_kph) ON CONFLICT DO NOTHING;
 GET DIAGNOSTICS v_inserted = ROW_COUNT;
 RETURN v_inserted=1;
END
$$;
ALTER FUNCTION zalgps_v2.accept_position(bytea,timestamptz,timestamptz,bytea,double precision,double precision,double precision) OWNER TO zalgps_functions;
REVOKE ALL ON FUNCTION zalgps_v2.accept_position(bytea,timestamptz,timestamptz,bytea,double precision,double precision,double precision) FROM PUBLIC;

CREATE FUNCTION zalgps_v2.latest_position(p_token_hash bytea)
RETURNS TABLE(fix_time timestamptz,received_at timestamptz,latitude double precision,longitude double precision,speed_kph double precision,tenant_id text,device_id text)
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, zalgps_v2, pg_temp
AS $$
DECLARE v_tenant text; v_device text;
BEGIN
 SELECT a.tenant_id,a.device_id INTO v_tenant,v_device FROM zalgps_v2.authenticate_device(p_token_hash) a;
 RETURN QUERY SELECT p.fix_time,p.received_at,p.latitude,p.longitude,p.speed_kph,p.tenant_id,p.device_id FROM zalgps_v2.positions p WHERE p.tenant_id=v_tenant AND p.device_id=v_device ORDER BY p.fix_time DESC,p.fingerprint DESC LIMIT 1;
END
$$;
ALTER FUNCTION zalgps_v2.latest_position(bytea) OWNER TO zalgps_functions;
REVOKE ALL ON FUNCTION zalgps_v2.latest_position(bytea) FROM PUBLIC;

CREATE FUNCTION zalgps_v2.enroll_device(p_tenant text,p_device text,p_token_hash bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, zalgps_v2, pg_temp
AS $$
BEGIN
 IF p_tenant !~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$' OR p_device !~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$' THEN RAISE EXCEPTION 'invalid device identifier'; END IF;
 INSERT INTO zalgps_v2.tenants(id) VALUES(p_tenant) ON CONFLICT DO NOTHING;
 INSERT INTO zalgps_v2.devices(tenant_id,device_id,token_hash) VALUES(p_tenant,p_device,p_token_hash);
END
$$;
ALTER FUNCTION zalgps_v2.enroll_device(text,text,bytea) OWNER TO zalgps_functions;
REVOKE ALL ON FUNCTION zalgps_v2.enroll_device(text,text,bytea) FROM PUBLIC;

CREATE FUNCTION zalgps_v2.rotate_device(p_tenant text,p_device text,p_token_hash bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, zalgps_v2, pg_temp
AS $$
BEGIN
 IF p_tenant !~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$' OR p_device !~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$' THEN RAISE EXCEPTION 'invalid device identifier'; END IF;
 UPDATE zalgps_v2.devices SET token_hash=p_token_hash,revoked=false WHERE tenant_id=p_tenant AND device_id=p_device;
 IF NOT FOUND THEN RAISE EXCEPTION 'device not found'; END IF;
END
$$;
ALTER FUNCTION zalgps_v2.rotate_device(text,text,bytea) OWNER TO zalgps_functions;
REVOKE ALL ON FUNCTION zalgps_v2.rotate_device(text,text,bytea) FROM PUBLIC;

CREATE FUNCTION zalgps_v2.revoke_device(p_tenant text,p_device text)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, zalgps_v2, pg_temp
AS $$
BEGIN
 IF p_tenant !~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$' OR p_device !~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$' THEN RAISE EXCEPTION 'invalid device identifier'; END IF;
 UPDATE zalgps_v2.devices SET revoked=true WHERE tenant_id=p_tenant AND device_id=p_device;
 IF NOT FOUND THEN RAISE EXCEPTION 'device not found'; END IF;
END
$$;
ALTER FUNCTION zalgps_v2.revoke_device(text,text) OWNER TO zalgps_functions;
REVOKE ALL ON FUNCTION zalgps_v2.revoke_device(text,text) FROM PUBLIC;
REVOKE CREATE ON SCHEMA zalgps_v2 FROM zalgps_functions;

GRANT USAGE ON SCHEMA zalgps_v2 TO zalgps_runtime,zalgps_provisioner;
GRANT SELECT ON zalgps_v2.schema_migrations TO zalgps_runtime;
GRANT EXECUTE ON FUNCTION zalgps_v2.accept_position(bytea,timestamptz,timestamptz,bytea,double precision,double precision,double precision),zalgps_v2.latest_position(bytea) TO zalgps_runtime;
GRANT EXECUTE ON FUNCTION zalgps_v2.enroll_device(text,text,bytea),zalgps_v2.rotate_device(text,text,bytea),zalgps_v2.revoke_device(text,text) TO zalgps_provisioner;
