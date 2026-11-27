CREATE SCHEMA travel;
REVOKE ALL ON SCHEMA travel FROM PUBLIC;

CREATE TABLE travel.catalog_revision (
    version bigint PRIMARY KEY CHECK (version > 0),
    provider text NOT NULL,
    environment text NOT NULL CHECK (environment IN ('synthetic', 'public')),
    updated_at timestamptz NOT NULL CHECK (isfinite(updated_at)),
    document bytea NOT NULL CHECK (octet_length(document) BETWEEN 1 AND 2097152),
    signature bytea NOT NULL CHECK (octet_length(signature) = 64)
);

CREATE TABLE travel.place (
    revision bigint NOT NULL REFERENCES travel.catalog_revision(version),
    id text NOT NULL CHECK (id ~ '^[a-z0-9][a-z0-9_-]{0,47}$'),
    region text NOT NULL CHECK (region ~ '^[a-z0-9][a-z0-9_-]{0,47}$'),
    name text NOT NULL CHECK (octet_length(name) BETWEEN 1 AND 180),
    address text NOT NULL CHECK (octet_length(address) BETWEEN 1 AND 360),
    longitude double precision CHECK (longitude BETWEEN -180 AND 180),
    latitude double precision CHECK (latitude BETWEEN -90 AND 90),
    source text NOT NULL CHECK (octet_length(source) BETWEEN 1 AND 180),
    updated_at timestamptz NOT NULL CHECK (isfinite(updated_at)),
    branch_ref uuid,
    capabilities text[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (revision, id),
    CHECK ((longitude IS NULL) = (latitude IS NULL)),
    CHECK ((branch_ref IS NULL) = (cardinality(capabilities) = 0)),
    CHECK (capabilities <@ ARRAY['booking','pickup']::text[])
);

CREATE TABLE travel.opening_interval (
    revision bigint NOT NULL,
    place_id text NOT NULL,
    weekday smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    timezone text NOT NULL,
    opens time NOT NULL,
    closes time NOT NULL,
    PRIMARY KEY (revision, place_id, weekday, opens),
    FOREIGN KEY (revision, place_id) REFERENCES travel.place(revision,id),
    CHECK (opens < closes)
);
