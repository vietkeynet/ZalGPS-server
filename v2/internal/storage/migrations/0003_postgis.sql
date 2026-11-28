-- DB-03: PostGIS must be provisioned per database by an authorized DBA.
DO $$
DECLARE
 v text;
 parts text[];
 major integer;
 minor integer;
 patch integer;
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM pg_catalog.pg_extension e
  JOIN pg_catalog.pg_namespace n ON n.oid=e.extnamespace
  WHERE e.extname='postgis' AND n.nspname='public'
 ) THEN
  RAISE EXCEPTION 'DB-03 requires PostGIS extension installed in public';
 END IF;
 SELECT e.extversion INTO v FROM pg_catalog.pg_extension e WHERE e.extname='postgis';
 parts := pg_catalog.regexp_match(v, '^([0-9]+)\.([0-9]+)\.([0-9]+)(\+[A-Za-z0-9.-]+)?$');
 IF parts IS NULL THEN
  RAISE EXCEPTION 'DB-03 cannot parse PostGIS version %', v;
 END IF;
 major := parts[1]::integer;
 minor := parts[2]::integer;
 patch := parts[3]::integer;
 IF (major, minor, patch) < (3, 5, 2)
    OR (major, minor, patch) >= (4, 0, 0) THEN
  RAISE EXCEPTION 'DB-03 supports PostGIS >= 3.5.2 and < 4.0.0; found %', v;
 END IF;
 IF EXISTS (
  SELECT 1 FROM pg_catalog.pg_namespace n CROSS JOIN LATERAL pg_catalog.aclexplode(n.nspacl) a
  WHERE n.oid='public'::pg_catalog.regnamespace
    AND a.grantee=0 AND a.privilege_type='CREATE'
 ) OR pg_catalog.has_schema_privilege('zalgps_runtime','public','CREATE')
   OR pg_catalog.has_schema_privilege('zalgps_provisioner','public','CREATE') THEN
  RAISE EXCEPTION 'DB-03 requires no PUBLIC, runtime, or provisioner CREATE on public';
 END IF;
END
$$;

ALTER TABLE zalgps_v2.positions
 ADD COLUMN location public.geography(Point,4326)
 GENERATED ALWAYS AS (
  public.ST_SetSRID(public.ST_MakePoint(longitude, latitude), 4326)::public.geography
 ) STORED;
CREATE INDEX positions_location_gist_idx
 ON zalgps_v2.positions USING gist(location);
