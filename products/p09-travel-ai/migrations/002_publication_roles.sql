CREATE INDEX place_region_catalog ON travel.place(revision,region,id);

DO $roles$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_public') THEN
        CREATE ROLE nu54_p09_public NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname='nu54_p09_publisher') THEN
        CREATE ROLE nu54_p09_publisher NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname IN ('nu54_p09_public','nu54_p09_publisher')
        AND (rolcanlogin OR rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication)) THEN
        RAISE EXCEPTION 'unsafe P09 capability role';
    END IF;
END
$roles$;

GRANT USAGE ON SCHEMA travel TO nu54_p09_public, nu54_p09_publisher;
GRANT SELECT ON travel.catalog_revision, travel.place, travel.opening_interval
    TO nu54_p09_public, nu54_p09_publisher;
GRANT INSERT ON travel.catalog_revision, travel.place, travel.opening_interval TO nu54_p09_publisher;
