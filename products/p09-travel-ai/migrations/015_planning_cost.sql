ALTER TABLE travel.place ADD COLUMN planning_cost text CHECK(planning_cost ~ '^(0|[1-9][0-9]{0,11})$');
ALTER TABLE travel.place ADD COLUMN planning_unit text CHECK(planning_unit IN ('KRW','TestUSDC'));
ALTER TABLE travel.place ADD CHECK((planning_cost IS NULL)=(planning_unit IS NULL));
