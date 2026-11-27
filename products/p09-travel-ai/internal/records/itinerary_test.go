package records

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/places"
	"github.com/jackc/pgx/v5"
)

func TestItinerarySourcesAndCAS(t *testing.T) {
	const planID = "43a668b5-142a-4e04-b178-bd736c30779d"
	at := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	catalog := places.Catalog{Version: 12, Places: []places.Place{{
		ID: "public-park", Name: "중앙 공원", Category: "attraction", Source: "Reviewed fixture", UpdatedAt: at,
		Opening: []places.OpeningInterval{{Weekday: int(at.Weekday()), Timezone: "Asia/Seoul", Opens: "10:00", Closes: "12:00"}},
	}}}
	plan := Plan{ID: planID, Consent: 1, Catalog: 12, Expires: at.Add(12 * time.Hour), Budget: "4000000",
		Stops: []StopInput{{Place: "public-park", Start: at.Add(time.Minute), End: at.Add(time.Hour)}}}
	got, err := validatePlan(catalog, plan, at)
	if err != nil || len(got.Sources) != 1 || got.Warnings[0] != "price_unconfirmed" || len(got.Details) != 1 ||
		got.Details[0].Name != "중앙 공원" || got.Details[0].Opening.Timezone != "Asia/Seoul" ||
		got.Details[0].Cost != nil {
		t.Fatalf("invalid approval preview: %v %v", got, err)
	}
	wrong := plan
	wrong.Catalog--
	if _, err := validatePlan(catalog, wrong, at); err == nil {
		t.Fatal("stale public catalog accepted")
	}
	wrong = plan
	wrong.Stops = []StopInput{{Place: "public-park", Start: at.Add(3 * time.Hour), End: at.Add(4 * time.Hour)}}
	if _, err := validatePlan(catalog, wrong, at); err == nil {
		t.Fatal("closed hours accepted")
	}
	dsn := os.Getenv("P09_PRIVATE_TEST_DSN")
	if dsn == "" {
		return
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(cfg.Database, "nu54_p09_private") ||
		!(strings.HasPrefix(cfg.Host, "/tmp/") || strings.HasPrefix(cfg.Host, "/private/tmp/")) {
		t.Fatal("use private temporary PG socket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	const actor = "07701b79-33b9-4abe-9437-6811d18376fe"
	const intent = "519afb86-4dd3-4d27-8b33-bd5fe02870f0"
	const edit = "59206817-33ba-4101-a70e-dfd7bdac2a2a"
	expires := time.Now().Add(time.Minute).UTC().Truncate(time.Microsecond)
	if _, err := admin.Exec(ctx, `INSERT INTO travel_private.policy VALUES ('recommendation',1,'p09',gen_random_uuid(),120,false,true)
		ON CONFLICT(purpose,version,recipient) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `SELECT travel_private.decide($1,'recommendation',0,1,'p09',NULL,true,$2,gen_random_uuid())`, actor, expires); err != nil {
		t.Fatal(err)
	}
	write := func(rev int, request string, digest uint8) (int, error) {
		t.Helper()
		var version int
		err := admin.QueryRow(ctx, `SELECT travel_private.apply_itinerary($1,$2,$3,1,12,$4,decode(repeat($5,32),'hex'),
		   decode(repeat('0a',32),'hex'),$6)`, actor, planID, rev, request, strings.Repeat(string(digest), 2), expires).Scan(&version)
		return version, err
	}
	if version, err := write(0, intent, '1'); err != nil || version != 1 {
		t.Fatalf("apply approval failed: %d %v", version, err)
	}
	if version, err := write(0, intent, '1'); err != nil || version != 1 {
		t.Fatalf("same approved request did not resume: %d %v", version, err)
	}
	if _, err := write(0, edit, '2'); err == nil {
		t.Fatal("stale intent overwrote itinerary")
	}
	if version, err := write(1, edit, '2'); err != nil || version != 2 {
		t.Fatalf("versioned apply unavailable: %d %v", version, err)
	}
	var current int
	var sealed []byte
	if err := admin.QueryRow(ctx, `SELECT revision,sealed FROM travel_private.read_itinerary($1,$2)`, actor, planID).Scan(&current, &sealed); err != nil || current != 2 {
		t.Fatal("separate cursor did not recover latest plan")
	}
	var count int
	foreign := "a30b8a13-45e7-4da8-b306-50d2b00d7670"
	for _, test := range []struct {
		subject string
		want    int
	}{{actor, 1}, {foreign, 0}} {
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM travel_private.list_itineraries($1)`, test.subject).Scan(&count); err != nil || count != test.want {
			t.Fatalf("scoped itinerary listing count=%d want=%d %v", count, test.want, err)
		}
	}
	if _, err := admin.Exec(ctx, `SELECT travel_private.decide($1,'recommendation',1,1,'p09',NULL,false,NULL,gen_random_uuid())`, actor); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM travel_private.list_itineraries($1)`, actor).Scan(&count); err != nil || count != 0 {
		t.Fatalf("withdrawn itinerary remained in list: count=%d %v", count, err)
	}
	var noSpend bool
	if err := admin.QueryRow(ctx, `SELECT to_regnamespace('ledger') IS NULL`).Scan(&noSpend); err != nil || !noSpend {
		t.Fatal("itinerary store shares financial authority")
	}
}
