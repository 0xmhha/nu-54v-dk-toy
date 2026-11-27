package commerce

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// opsd and the database refuse the same hidden characters: every code point is asked of both. A character one of
// them took and the other refused would be text a store can save through one door and not the other.
func TestPostgresHiddenCharactersAreTheSameInOpsdAndTheDatabase(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(config.Database, "nu54_commerce_") {
		t.Fatal("use disposable commerce database")
	}
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Minute)
	defer stop()
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	// Every code point a text can hold: all but zero and the surrogates.
	rows, err := admin.Query(ctx, `SELECT i FROM generate_series(1,1114111) i
		WHERE (i<55296 OR i>57343) AND NOT ledger.store_text_plain('a'||chr(i)) ORDER BY i`)
	if err != nil {
		t.Fatal(err)
	}
	refused := map[rune]bool{}
	for rows.Next() {
		var point int32
		if err := rows.Scan(&point); err != nil {
			t.Fatal(err)
		}
		refused[rune(point)] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(refused) < 4000 || !refused[0x2065] || !refused[0x3164] || refused['a'] || refused[' '] || refused['가'] {
		t.Fatalf("the database refuses %d code points; the list is not the one this test knows", len(refused))
	}
	var apart []string
	for point := rune(1); point <= 0x10FFFF; point++ {
		if point >= 0xD800 && point <= 0xDFFF {
			continue
		}
		if hidden := !visibleLine("a" + string(point)); hidden != refused[point] {
			apart = append(apart, fmt.Sprintf("U+%04X (opsd refuses: %v, the database refuses: %v)", point, hidden, refused[point]))
		}
	}
	if len(apart) != 0 {
		t.Fatalf("%d code points are judged differently: %s", len(apart), strings.Join(apart[:min(len(apart), 20)], ", "))
	}
	// The rule about spaces is the same too: a text of ordinary spaces alone is not a visible line, an empty one is.
	for _, text := range []string{"", " ", "   ", " a ", "a b"} {
		var plain bool
		if err := admin.QueryRow(ctx, "SELECT ledger.store_text_plain($1)", text).Scan(&plain); err != nil || plain != visibleLine(text) {
			t.Fatalf("%q: the database says %v, opsd %v (%v)", text, plain, visibleLine(text), err)
		}
	}
}
