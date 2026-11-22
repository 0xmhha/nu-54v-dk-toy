package places

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	pool      *pgxpool.Pool
	publisher bool
}

func Connect(ctx context.Context, dsn string, publisher bool) (*Database, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid P09 database settings")
	}
	config.ConnConfig.RuntimeParams["statement_timeout"] = "8s"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("P09 database connection unavailable")
	}
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT
 EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND NOT(rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls))
 AND pg_has_role(current_user, $1, 'USAGE')
 AND NOT has_schema_privilege(current_user,'travel','CREATE')
 AND has_table_privilege(current_user,'travel.place','INSERT') = $2`,
		map[bool]string{true: "nu54_p09_publisher", false: "nu54_p09_public"}[publisher], publisher).Scan(&allowed)
	if err != nil || !allowed {
		pool.Close()
		return nil, errors.New("P09 database capability or migration unavailable")
	}
	return &Database{pool, publisher}, nil
}

func (db *Database) Close() { db.pool.Close() }

func (db *Database) Publish(ctx context.Context, key ed25519.PublicKey, raw, signature []byte, now time.Time) error {
	if !db.publisher {
		return errors.New("catalog publication unavailable")
	}
	catalog, err := parse(key, sealed{raw, signature})
	if err != nil {
		return err
	}
	if catalog.UpdatedAt.After(now) {
		return errors.New("catalog claimed a future update")
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(900017)`); err != nil {
		return err
	}
	var previous uint64
	var previousTime time.Time
	err = tx.QueryRow(ctx, `SELECT version,updated_at FROM travel.catalog_revision ORDER BY version DESC LIMIT 1`).Scan(&previous, &previousTime)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if previous >= catalog.Version || previousTime.After(catalog.UpdatedAt) {
		return errors.New("catalog version or source time regressed")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO travel.catalog_revision VALUES($1,$2,$3,$4,$5,$6)`,
		catalog.Version, catalog.Provider, catalog.Environment, catalog.UpdatedAt, raw, signature); err != nil {
		return err
	}
	for _, place := range catalog.Places {
		capabilities := place.Capabilities
		if capabilities == nil {
			capabilities = []string{}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO travel.place VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::uuid,$11,$12,$13,$14,$15)`,
			catalog.Version, place.ID, place.Region, place.Name, place.Address, place.Longitude, place.Latitude,
			place.Source, place.UpdatedAt, place.BranchRef, capabilities, place.RegionName, place.Category, place.PlanningCost, place.PlanningUnit); err != nil {
			return err
		}
		for _, item := range place.Opening {
			if _, err = tx.Exec(ctx, `INSERT INTO travel.opening_interval VALUES($1,$2,$3,$4,$5::text::time,$6::text::time)`,
				catalog.Version, place.ID, item.Weekday, item.Timezone, item.Opens, item.Closes); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func (db *Database) Published(ctx context.Context, key ed25519.PublicKey) (Catalog, error) {
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Catalog{}, err
	}
	defer tx.Rollback(ctx)
	var actual Catalog
	var input sealed
	err = tx.QueryRow(ctx, `SELECT version,provider,environment,updated_at,document,signature FROM travel.catalog_revision
		ORDER BY version DESC LIMIT 1`).Scan(&actual.Version, &actual.Provider, &actual.Environment,
		&actual.UpdatedAt, &input.Document, &input.Signature)
	if err != nil {
		return Catalog{}, err
	}
	expected, err := parse(key, input)
	if err != nil {
		return Catalog{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id,region,name,address,longitude,latitude,source,updated_at,coalesce(branch_ref::text,''),capabilities,region_name,category,planning_cost,planning_unit
		FROM travel.place WHERE revision=$1 ORDER BY id`, actual.Version)
	if err != nil {
		return Catalog{}, err
	}
	for rows.Next() {
		var p Place
		if err = rows.Scan(&p.ID, &p.Region, &p.Name, &p.Address, &p.Longitude, &p.Latitude, &p.Source, &p.UpdatedAt,
			&p.BranchRef, &p.Capabilities, &p.RegionName, &p.Category, &p.PlanningCost, &p.PlanningUnit); err != nil {
			rows.Close()
			return Catalog{}, err
		}
		actual.Places = append(actual.Places, p)
	}
	if err = rows.Err(); err != nil {
		return Catalog{}, err
	}
	opening, err := tx.Query(ctx, `SELECT place_id,weekday,timezone,to_char(opens,'HH24:MI'),to_char(closes,'HH24:MI')
		FROM travel.opening_interval WHERE revision=$1 ORDER BY place_id,weekday,opens`, actual.Version)
	if err != nil {
		return Catalog{}, err
	}
	index := make(map[string]int, len(actual.Places))
	for i, entry := range actual.Places {
		index[entry.ID] = i
	}
	for opening.Next() {
		var item OpeningInterval
		var id string
		if err = opening.Scan(&id, &item.Weekday, &item.Timezone, &item.Opens, &item.Closes); err != nil {
			opening.Close()
			return Catalog{}, err
		}
		at, ok := index[id]
		if !ok {
			opening.Close()
			return Catalog{}, errors.New("opening interval without published place")
		}
		actual.Places[at].Opening = append(actual.Places[at].Opening, item)
	}
	if err = opening.Err(); err != nil {
		return Catalog{}, err
	}
	normalize := func(c Catalog) []byte {
		c.UpdatedAt = c.UpdatedAt.UTC()
		if len(c.Places) == 0 {
			c.Places = []Place{}
		}
		for i := range c.Places {
			c.Places[i].UpdatedAt = c.Places[i].UpdatedAt.UTC()
		}
		formatted, _ := json.Marshal(c)
		return formatted
	}
	if !bytes.Equal(normalize(actual), normalize(expected)) {
		return Catalog{}, errors.New("published rows differ from the approved document")
	}
	return actual, tx.Commit(ctx)
}
