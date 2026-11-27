ALTER TABLE travel.place
    ADD COLUMN region_name text NOT NULL DEFAULT '' CHECK (octet_length(region_name)<=120),
    ADD COLUMN category text NOT NULL DEFAULT '' CHECK (category IN ('','restaurant','attraction'));
